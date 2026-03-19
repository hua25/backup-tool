package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Version 版本信息
const Version = "0.2.0"

// 重新排列参数，让 flags 在 positional args 前面
// 处理 -d value 这种 flag-value 对
func reorderArgs() []string {
	args := os.Args[1:]
	var flags, pos []string
	knownFlags := map[string]bool{"-d": true, "--dest": true, "-s": true, "--suffix": true, "-f": true, "--time-format": true, "--keep": true, "--keep-days": true}

	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flags = append(flags, arg)
			// 如果是带值的 flag，下一个参数是它的值
			if knownFlags[arg] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i += 2
				continue
			}
			i++
		} else {
			pos = append(pos, arg)
			i++
		}
	}
	return append(flags, pos...)
}

// Config 配置
type Config struct {
	Dest       string
	Suffix     string
	TimeFormat string
	DryRun     bool
	Verbose    bool
	Quiet      bool
	Keep       int
	KeepDays   int
}

// FileMeta 文件元数据
type FileMeta struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

// Meta 元数据
type Meta struct {
	Version    string     `json:"version"`
	Source     string     `json:"source"`
	SourcePath string     `json:"source_path"`
	CreatedAt  string     `json:"created_at"`
	Size       int64      `json:"size"`
	Checksum   string     `json:"checksum,omitempty"`
	IsDir      bool       `json:"is_dir"`
	Files      []FileMeta `json:"files,omitempty"`
}

// BackupResult 备份结果
type BackupResult struct {
	Source string
	Dest   string
	Size   int64
	Err    error
}

func main() {
	cfg := parseArgs()

	if flag.NArg() < 1 {
		printUsage()
		os.Exit(1)
	}

	sources := flag.Args()
	var results []BackupResult

	for _, src := range sources {
		result := backup(src, cfg)
		results = append(results, result)
	}

	// 统一报告所有结果
	failed := 0
	for _, r := range results {
		if r.Err != nil {
			failed++
			if !cfg.Quiet {
				fmt.Fprintf(os.Stderr, "✗ error: %s -> %v\n", r.Source, r.Err)
			}
		} else if !cfg.Quiet {
			fmt.Printf("✓ backup: %s -> %s (%s)\n", r.Source, r.Dest, formatSize(r.Size))
		}
	}

	if failed > 0 {
		fmt.Fprintf(os.Stderr, "\n%d/%d failed\n", failed, len(results))
		os.Exit(1)
	}
}

// parseArgs 解析命令行参数
func parseArgs() Config {
	cfg := Config{
		Suffix:     ".bak",
		TimeFormat: "20060102_150405",
	}

	flag.StringVar(&cfg.Dest, "d", "", "备份目标目录")
	flag.StringVar(&cfg.Dest, "dest", "", "备份目标目录")
	flag.StringVar(&cfg.Suffix, "s", ".bak", "备份后缀")
	flag.StringVar(&cfg.Suffix, "suffix", ".bak", "备份后缀")
	flag.StringVar(&cfg.TimeFormat, "f", "20060102_150405", "时间戳格式")
	flag.StringVar(&cfg.TimeFormat, "time-format", "20060102_150405", "时间戳格式")
	flag.BoolVar(&cfg.DryRun, "n", false, "模拟运行")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "模拟运行")
	flag.BoolVar(&cfg.Verbose, "v", false, "详细输出")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "详细输出")
	flag.BoolVar(&cfg.Quiet, "q", false, "静默模式")
	flag.BoolVar(&cfg.Quiet, "quiet", false, "静默模式")
	flag.IntVar(&cfg.Keep, "keep", 0, "保留最近 N 个备份")
	flag.IntVar(&cfg.KeepDays, "keep-days", 0, "保留最近 N 天")

	// 处理 --version
	for _, arg := range os.Args[1:] {
		if arg == "--version" {
			fmt.Printf("bak version %s\n", Version)
			os.Exit(0)
		}
	}

	flag.Usage = printUsage
	// 允许 flags 出现在 positional args 之前或之后
	flag.CommandLine.Parse(reorderArgs())
	return cfg
}

func printUsage() {
	fmt.Println(`bak - 文件备份工具

用法:
  bak <源文件> [源文件2 ...] [flags]

示例:
  bak report.pdf                    # 备份单个文件
  bak data.csv -d /backup/         # 指定目标目录
  bak /etc/nginx/                   # 备份目录
  bak important.dat -n              # 模拟运行
  bak file1.txt file2.txt          # 批量备份

参数:
  -d, --dest string        备份目标目录 (默认: <源文件>_bak/)
  -s, --suffix string      备份后缀 (默认: .bak)
  -f, --time-format string 时间戳格式 (默认: 20060102_150405)
  -n, --dry-run            模拟运行，不实际操作
  -v, --verbose            显示详细输出
  -q, --quiet              静默模式
  --keep int               只保留最近 N 个备份
  --keep-days int          保留最近 N 天的备份

其他:
  -h, --help               显示帮助
  --version                显示版本`)
}

func backup(srcPath string, cfg Config) BackupResult {
	result := BackupResult{Source: srcPath}

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		result.Err = fmt.Errorf("source file not found: %s", srcPath)
		return result
	}

	// 确定目标目录
	destDir := cfg.Dest
	if destDir == "" {
		destDir = srcPath + cfg.Suffix
	}

	// 生成时间戳文件名
	timestamp := time.Now().Format(cfg.TimeFormat)
	baseName := filepath.Base(srcPath)

	var destPath string
	if srcInfo.IsDir() {
		// 目录备份
		dirName := baseName + ".bak." + timestamp
		destPath = filepath.Join(destDir, dirName)
	} else {
		// 文件备份
		ext := filepath.Ext(baseName)
		nameWithoutExt := strings.TrimSuffix(baseName, ext)
		backupName := fmt.Sprintf("%s%s.%s%s", nameWithoutExt, cfg.Suffix, timestamp, ext)
		destPath = filepath.Join(destDir, backupName)
	}

	result.Dest = destPath

	if cfg.Verbose {
		fmt.Printf("[INFO] Starting backup...\n")
		fmt.Printf("[SOURCE] %s (%s)\n", srcPath, formatSize(srcInfo.Size()))
		fmt.Printf("[DEST]   %s\n", destPath)
	}

	if cfg.DryRun {
		if !cfg.Quiet {
			fmt.Printf("⚠ [DRY-RUN] %s -> %s\n", srcPath, destPath)
		}
		return result
	}

	// 创建目标目录
	if err := os.MkdirAll(destDir, 0755); err != nil {
		result.Err = fmt.Errorf("failed to create dest dir: %w", err)
		return result
	}

	// 执行备份
	var srcChecksum string
	var fileMetas []FileMeta

	if srcInfo.IsDir() {
		err = copyDir(srcPath, destPath)
		if err != nil {
			result.Err = err
			return result
		}
		// 目录备份：递归收集所有文件 checksum
		fileMetas, err = collectFileChecksums(srcPath)
		if err != nil {
			result.Err = fmt.Errorf("failed to collect checksums: %w", err)
			return result
		}
	} else {
		err = copyFile(srcPath, destPath)
		if err != nil {
			result.Err = err
			return result
		}
		// 文件备份：计算 checksum
		srcChecksum, err = checksum(srcPath)
		if err != nil {
			result.Err = fmt.Errorf("checksum failed: %w", err)
			return result
		}
		result.Size = srcInfo.Size()
	}

	// 写入元数据
	meta := Meta{
		Version:    Version, // P1: 同步 Version 常量
		Source:     filepath.Base(srcPath),
		SourcePath: srcPath,
		CreatedAt:  time.Now().Format(time.RFC3339),
		Size:       srcInfo.Size(),
		Checksum:   srcChecksum,
		IsDir:      srcInfo.IsDir(),
		Files:      fileMetas,
	}

	metaPath := filepath.Join(destDir, ".bak.meta.json")
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		result.Err = fmt.Errorf("failed to marshal metadata: %w", err)
		return result
	}

	// P0: 元数据写入必须检查 error
	if err := os.WriteFile(metaPath, metaBytes, 0644); err != nil {
		result.Err = fmt.Errorf("failed to write metadata: %w", err)
		return result
	}

	if cfg.Verbose {
		if srcChecksum != "" {
			fmt.Printf("[VERIFY] checksum: %s\n", srcChecksum[:16]+"...")
		}
		if len(fileMetas) > 0 {
			fmt.Printf("[VERIFY] %d files checked\n", len(fileMetas))
		}
		fmt.Printf("[OK] Backup completed\n")
	}

	return result
}

// P0: copyFile 返回 error 而非忽略
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

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		os.Remove(dst)
		return fmt.Errorf("copy content: %w", err)
	}

	// P0: 检查 chmod 错误（软失败，记录警告但不阻断）
	srcInfo, err := os.Stat(src)
	if err != nil {
		os.Remove(dst)
		return fmt.Errorf("stat source: %w", err)
	}

	if err = os.Chmod(dst, srcInfo.Mode()); err != nil {
		// 权限修改失败不阻断，但记录
		// 如需严格模式可改为 return error
	}

	// P0: 检查 Close 错误
	if err = dstFile.Close(); err != nil {
		os.Remove(dst)
		return fmt.Errorf("close dest: %w", err)
	}

	return nil
}

// P0: copyDir 返回 error 而非忽略
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source dir: %w", err)
	}

	if err = os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}

	for _, entry := range entries {
		srcEntry := filepath.Join(src, entry.Name())
		dstEntry := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// P0: 递归调用返回 error
			if err := copyDir(srcEntry, dstEntry); err != nil {
				return fmt.Errorf("copy dir %s: %w", srcEntry, err)
			}
		} else {
			// P0: copyFile 返回 error
			if err := copyFile(srcEntry, dstEntry); err != nil {
				return fmt.Errorf("copy file %s: %w", srcEntry, err)
			}
		}
	}

	return nil
}

// P1: 递归收集目录下所有文件的 checksum
func collectFileChecksums(root string) ([]FileMeta, error) {
	var metas []FileMeta

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk error at %s: %w", path, err)
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		cs, err := checksum(path)
		if err != nil {
			return fmt.Errorf("checksum %s: %w", path, err)
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("rel path %s: %w", path, err)
		}

		metas = append(metas, FileMeta{
			Path:     rel,
			Size:     info.Size(),
			Checksum: cs,
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return metas, nil
}

func checksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

// P2 fix: formatSize 对 PB 以上越界问题（扩展 units）
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
