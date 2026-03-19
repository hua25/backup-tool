# backup-tool 优化方案文档

基于 v0.1.0 代码评审，整理以下优化方案。

---

## 已完成优化 (v0.2.0)

### ✅ P0 - 错误处理完善

| 状态 | 项目 | 说明 |
|------|------|------|
| ✅ | copyFile 返回 error | os.Chmod 软失败记录警告，dstFile.Close 检查错误 |
| ✅ | copyDir 返回 error | 递归调用传递错误，每层调用都返回错误 |
| ✅ | 元数据写入检查 | os.WriteFile 返回值必须判断 |
| ✅ | BackupResult 类型 | 统一的结果结构，携带 Source/Dest/Size/Err |

### ✅ P1 - 备份元数据增强

| 状态 | 项目 | 说明 |
|------|------|------|
| ✅ | Version 常量同步 | meta.Version = Version (0.2.0) |
| ✅ | 目录逐文件 checksum | collectFileChecksums 递归收集所有文件 |
| ✅ | 批量任务错误聚合 | main 函数收集所有结果，统一报告 |
| ✅ | IsDir 字段 | Meta 新增 is_dir 字段标识备份类型 |
| ✅ | Files 字段 | Meta 新增 files 数组存储目录内文件信息 |

### ✅ P2 - 健壮性修复

| 状态 | 项目 | 说明 |
|------|------|------|
| ✅ | formatSize 越界修复 | units 扩展到 "BKMGTPE"，支持 EB |

---

## 优化后元数据结构

```json
{
  "version": "0.2.0",
  "source": "bak-test",
  "source_path": "/path/to/bak-test",
  "created_at": "2026-03-19T15:29:47Z",
  "size": 4096,
  "checksum": "",
  "is_dir": true,
  "files": [
    {
      "path": "file1.txt",
      "size": 6,
      "checksum": "sha256:ecdc5536f73bdae8816f0ea40726ef5e9b810d914493075903bb90623d97b1d8"
    },
    {
      "path": "subdir/file2.txt",
      "size": 6,
      "checksum": "sha256:67ee5478eaadb034ba59944eb977797b49ca6aa8d3574587f36ebcbeeb65f70e"
    }
  ]
}
```

---

## 批量备份结果示例

```
$ bak file1.txt nonexistent.txt /data

✓ backup: file1.txt -> file1.txt.bak/file1.bak.20260319_152954.txt (6 B)
✗ error: nonexistent.txt -> source file not found: nonexistent.txt
✓ backup: /data -> /data.bak/data.bak.20260319_152958/ (0 B)

1/3 failed
```

---

## 待完成优化

### P2 - 健壮性增强

| 状态 | 项目 | 说明 |
|------|------|------|
| ⬜ | 磁盘空间检查 | 备份前预估所需空间，不足则提前报错 |
| ⬜ | 递归深度限制 | 添加 maxDepth 防止路径过深导致栈溢出 |

### P3 - 功能增强

| 状态 | 项目 | 说明 |
|------|------|------|
| ⬜ | 压缩备份 | gzip 压缩输出，节省空间 |
| ⬜ | 增量备份 | 基于 mtime + size 快速判断变化 |
| ⬜ | 恢复命令 | bak restore 恢复到原位置或新位置 |

### P4 - 用户体验

| 状态 | 项目 | 说明 |
|------|------|------|
| ⬜ | 进度提示 | 大型文件显示复制进度 |
| ⬜ | `--` 分隔符 | 支持 bak -- file.txt 明确分隔参数 |

---

## 变更记录

| 版本 | 日期 | 变更 |
|------|------|------|
| v0.2.0 | 2026-03-19 | P0/P1/P2 优化完成：错误处理完善、元数据增强、formatSize 修复 |
| v0.1.0 | 2026-03-19 | 初始版本：基础备份功能 |
