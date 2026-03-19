# backup-tool 优化方案文档

基于 v0.1.0 代码评审，整理以下优化方案。

## 一、错误处理优化

### 1.1 当前问题

以下位置忽略了返回的 error，存在静默失败风险：

| 位置 | 问题 |
|------|------|
| `copyFile` L: os.Chmod | 权限复制失败时被忽略 |
| `copyDir` L: 递归调用 | 内部 copyFile/copyDir 错误未收集 |
| `backup` L: 元数据写入 | `os.WriteFile` 错误未判断 |
| `backup` L: srcInfo 复用 | copyDir 后 srcInfo 可能过期 |

### 1.2 优化方案

```go
// 1. 统一错误收集
type BackupError struct {
    Path   string
    Err    error
    IsDir  bool
}

func (e *BackupError) Error() string {
    return fmt.Sprintf("%s: %v", e.Path, e.Err)
}

// 2. copyFile 增强
func copyFile(src, dst string) error {
    srcFile, err := os.Open(src)
    if err != nil {
        return fmt.Errorf("open source: %w", err)
    }
    defer srcFile.Close()

    dstFile, err := os.Create(dst)
    if err != nil {
        return fmt.Errorf("create dest: %w", err)
    }
    defer dstFile.Close()

    if _, err = io.Copy(dstFile, srcFile); err != nil {
        os.Remove(dst)
        return fmt.Errorf("copy content: %w", err)
    }

    // 传递错误而非忽略
    srcInfo, err := os.Stat(src)
    if err != nil {
        os.Remove(dst)
        return fmt.Errorf("stat source: %w", err)
    }

    if err = os.Chmod(dst, srcInfo.Mode()); err != nil {
        // 软失败：权限可能因所有者问题无法修改，记录但不阻断
        log.Printf("[WARN] chmod failed for %s: %v", dst, err)
    }

    return dstFile.Close()
}

// 3. copyDir 返回错误列表
func copyDir(src, dst string) []error {
    var errs []error

    srcInfo, err := os.Stat(src)
    if err != nil {
        return append(errs, fmt.Errorf("stat source dir: %w", err))
    }

    if err = os.MkdirAll(dst, srcInfo.Mode()); err != nil {
        return append(errs, fmt.Errorf("create dest dir: %w", err))
    }

    entries, err := os.ReadDir(src)
    if err != nil {
        return append(errs, fmt.Errorf("read dir: %w", err))
    }

    for _, entry := range entries {
        srcEntry := filepath.Join(src, entry.Name())
        dstEntry := filepath.Join(dst, entry.Name())

        if entry.IsDir() {
            subErrs := copyDir(srcEntry, dstEntry)
            errs = append(errs, subErrs...)
        } else {
            if err := copyFile(srcEntry, dstEntry); err != nil {
                errs = append(errs, fmt.Errorf("%s: %w", srcEntry, err))
            }
        }
    }

    return errs
}

// 4. 元数据写入必须检查
metaPath := filepath.Join(destDir, ".bak.meta.json")
if err := os.WriteFile(metaPath, metaBytes, 0644); err != nil {
    return fmt.Errorf("write metadata: %w", err)
}
```

### 1.3 批量任务错误聚合

```go
type BackupResult struct {
    Source  string
    Dest    string
    Err     error
    Size    int64
}

func main() {
    cfg := parseArgs()
    // ...
    var results []BackupResult

    for _, src := range sources {
        result := BackupResult{Source: src}
        if err := backup(src, cfg); err != nil {
            result.Err = err
        }
        results = append(results, result)
    }

    // 统一报告
    failed := 0
    for _, r := range results {
        if r.Err != nil {
            failed++
            fmt.Fprintf(os.Stderr, "✗ failed: %s -> %v\n", r.Source, r.Err)
        }
    }

    if failed > 0 {
        fmt.Fprintf(os.Stderr, "\n%d/%d failed\n", failed, len(results))
        os.Exit(1)
    }
}
```

---

## 二、参数与默认行为优化

### 2.1 当前问题

- `destDir` 默认 `srcPath + cfg.Suffix`，路径很长时嵌套深
- 不支持 `--` 分割符，复杂场景有歧义
- `-d` 值必须有，否则报错

### 2.2 优化方案

```go
// 更智能的默认目标
func resolveDestDir(srcPath, userDest string, suffix string) string {
    if userDest != "" {
        return userDest
    }

    // 用户提供的是目录 → 放里面
    if info, err := os.Stat(userDest); err == nil && info.IsDir() {
        return userDest
    }

    // 否则默认同目录 xxx.bak/
    return srcPath + suffix
}

// 支持 -- 分隔符
func parseArgs() Config {
    // 手动解析，支持 -- 分割
    args := os.Args[1:]
    separatorIdx := -1
    for i, arg := range args {
        if arg == "--" {
            separatorIdx = i
            break
        }
    }

    var flags, pos []string
    if separatorIdx >= 0 {
        flags = args[:separatorIdx]
        pos = args[separatorIdx+1:]
    } else {
        flags, pos = splitFlagsAndPos(args)
    }
    // ...
}
```

### 2.3 目录备份默认命名优化

```
# 当前（嵌套深）
源: /data/project
→ /data/project.bak/data.project.bak.20260319_143000/

# 优化后（扁平）
→ /backup/data.project.20260319_143000/
```

---

## 三、备份元数据优化

### 3.1 当前问题

- `meta.Version` 写死 "1.0"，不同步 `Version` 常量
- 目录备份无逐文件校验和

### 3.2 优化方案

```go
type Meta struct {
    Version     string            `json:"version"`
    Source      string            `json:"source"`
    SourcePath  string            `json:"source_path"`
    CreatedAt   string            `json:"created_at"`
    Size        int64             `json:"size"`
    Checksum    string            `json:"checksum,omitempty"`  // 文件时填
    IsDir       bool              `json:"is_dir"`
    Files       []FileMeta        `json:"files,omitempty"`      // 目录时填
}

type FileMeta struct {
    Path     string `json:"path"`
    Size     int64  `json:"size"`
    Checksum string `json:"checksum"`
}

// 目录备份时递归收集
func (m *Meta) collectFileChecksums(root string) error {
    return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
        if err != nil || info.IsDir() {
            return nil
        }
        cs, err := checksum(path)
        if err != nil {
            return err
        }
        rel, _ := filepath.Rel(root, path)
        m.Files = append(m.Files, FileMeta{
            Path:     rel,
            Size:     info.Size(),
            Checksum: cs,
        })
        return nil
    })
}
```

### 3.3 版本常量同步

```go
// main.go
const Version = "0.2.0"

// Meta 初始化时引用
meta := Meta{
    Version:   Version,  // 而非写死的 "1.0"
    // ...
}
```

---

## 四、健壮性增强

### 4.1 递归深度限制

```go
const maxDepth = 100

func copyDirDepth(src, dst string, depth int) error {
    if depth > maxDepth {
        return fmt.Errorf("max depth %d exceeded: %s", maxDepth, src)
    }
    // ... 递归时 depth+1
}
```

### 4.2 formatSize 容量越界修复

```go
func formatSize(size int64) string {
    const unit = 1024
    units := "BKMGTPE" // 扩展到 E
    if size < unit {
        return fmt.Sprintf("%d B", size)
    }
    div, exp := int64(unit), 0
    for n := size / unit; n >= unit && exp < len(units)-2; n /= unit {
        div *= unit
        exp++
    }
    return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), units[exp+1])
}
```

### 4.3 磁盘空间检查

```go
func checkDiskSpace(srcPath, destDir string) error {
    var dstFs syscall.Statfs_t
    dest := destDir
    if dest == "" {
        dest = srcPath + ".bak"
    }
    if err := syscall.Statfs(filepath.Dir(dest), &dstFs); err != nil {
        return fmt.Errorf("statfs: %w", err)
    }

    srcSize, _ := dirSize(srcPath)
    if int64(dstFs.Bfree) < srcSize {
        return fmt.Errorf("insufficient disk space: need %s, have %s",
            formatSize(srcSize), formatSize(int64(dstFs.Bfree)*int64(dstFs.Bsize)))
    }
    return nil
}
```

---

## 五、用户体验优化

### 5.1 正常输出增强

```go
// 默认输出（无 -v 时）也提供有用信息
if !cfg.Quiet {
    if cfg.DryRun {
        fmt.Printf("⚠ [DRY-RUN] %s -> %s\n", src, dest)
    } else {
        fmt.Printf("✓ %s -> %s (%s)\n", src, dest, formatSize(size))
    }
}
```

### 5.2 进度提示（大型文件）

```go
func copyFileWithProgress(src, dst string, verbose bool) error {
    srcFile, _ := os.Open(src)
    defer srcFile.Close()

    dstFile, _ := os.Create(dst)
    defer dstFile.Close()

    total, _ := srcFile.Seek(0, io.SeekEnd)
    srcFile.Seek(0, 0)

    if verbose {
        fmt.Printf("[PROGRESS] ")
    }

    buf := make([]byte, 32*1024)
    var copied int64
    for {
        n, err := srcFile.Read(buf)
        if n > 0 {
            dstFile.Write(buf[:n])
            copied += int64(n)
            if verbose && total > 0 {
                pct := float64(copied) / float64(total) * 100
                fmt.Printf("\r[%s] %.0f%%", formatSize(copied), pct)
            }
        }
        if err == io.EOF {
            break
        }
    }
    if verbose {
        fmt.Println()
    }
    return nil
}
```

---

## 六、功能增强（v0.2 / v0.3）

### 6.1 压缩备份

```bash
bak file.txt -z           # gzip 压缩
bak file.txt -z --level 6 # 压缩级别
```

```go
// 输出: file.txt.bak/20260319.gz
// 解压恢复: bak restore file.txt.bak/xxx.gz
```

### 6.2 增量备份

```bash
# 首次全量
bak /data -i --tag v1
# 输出: /data.bak/v1.20260319_143000/

# 第二次增量（只备份变化）
bak /data -i --since /data.bak/v1.20260319_143000/
# 输出: /data.bak/v1.20260319_143000.delta/
```

实现：基于文件 mtime + size 快速判断变化，不改则跳过。

### 6.3 恢复命令

```bash
bak restore /backup/data.bak/20260319_143000/  # 恢复到原位置
bak restore /backup/data.bak/20260319_143000/ -d /new/path/  # 恢复到新位置
```

---

## 七、优化优先级

| 优先级 | 项目 | 影响 |
|--------|------|------|
| P0 | 错误处理完善（收集+聚合） | 可靠性 |
| P0 | 元数据写入检查 | 数据完整性 |
| P1 | 版本常量同步 | 可维护性 |
| P1 | 目录备份逐文件 checksum | 数据完整性 |
| P1 | 批量任务错误聚合 | 用户体验 |
| P2 | 磁盘空间检查 | 健壮性 |
| P2 | 递归深度限制 | 安全性 |
| P3 | 压缩备份 | 功能增强 |
| P3 | 增量备份 | 功能增强 |
| P4 | 进度提示 | 用户体验 |

---

## 八、待办清单

- [ ] 重构 `copyFile`/`copyDir` 返回 error 而非忽略
- [ ] 实现 `BackupError` 类型和批量错误聚合
- [ ] 修复 `formatSize` 越界问题
- [ ] 同步 `meta.Version` 与 `Version` 常量
- [ ] 目录备份时递归收集文件 checksum
- [ ] 添加磁盘空间预检查
- [ ] 添加 `--` 分隔符支持
- [ ] 实现压缩备份（gzip）
- [ ] 实现增量备份
- [ ] 实现 `bak restore` 恢复命令
