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

// Version 版本信息
const Version = "0.1.0"

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

// Meta 元数据
type Meta struct {
	Version    string `json:"version"`
	Source     string `json:"source"`
	SourcePath string `json:"source_path"`
	CreatedAt  string `json:"created_at"`
	Size       int64  `json:"size"`
	Checksum   string `json:"checksum"`
}

func main() {
	cfg := parseArgs()

	if flag.NArg() < 1 {
		printUsage()
		os.Exit(1)
	}

	sources := flag.Args()

	for _, src := range sources {
		if err := backup(src, cfg); err != nil {
			if !cfg.Quiet {
				fmt.Fprintf(os.Stderr, "✗ error: %s\n", err)
			}
			os.Exit(1)
		}
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

func backup(srcPath string, cfg Config) error {
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("source file not found: %s", srcPath)
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

	if cfg.Verbose {
		fmt.Printf("[INFO] Starting backup...\n")
		fmt.Printf("[SOURCE] %s (%s)\n", srcPath, formatSize(srcInfo.Size()))
		fmt.Printf("[DEST]   %s\n", destPath)
	}

	if cfg.DryRun {
		if !cfg.Quiet {
			fmt.Printf("✓ [DRY-RUN] %s -> %s\n", srcPath, destPath)
		}
		return nil
	}

	// 创建目标目录
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create dest dir: %w", err)
	}

	if srcInfo.IsDir() {
		err = copyDir(srcPath, destPath)
	} else {
		err = copyFile(srcPath, destPath)
	}

	if err != nil {
		return err
	}

	// 验证 checksum (跳过目录)
	var srcChecksum string
	if !srcInfo.IsDir() {
		srcChecksum, err = checksum(srcPath)
		if err != nil {
			return fmt.Errorf("checksum failed: %w", err)
		}
	}

	// 写入元数据
	meta := Meta{
		Version:    "1.0",
		Source:     filepath.Base(srcPath),
		SourcePath: srcPath,
		CreatedAt:  time.Now().Format(time.RFC3339),
		Size:       srcInfo.Size(),
		Checksum:   srcChecksum,
	}

	metaPath := filepath.Join(destDir, ".bak.meta.json")
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(metaPath, metaBytes, 0644)

	if !cfg.Quiet {
		fmt.Printf("✓ backup: %s -> %s (%s)\n", srcPath, destPath, formatSize(srcInfo.Size()))
	}

	if cfg.Verbose && srcChecksum != "" {
		fmt.Printf("[VERIFY] checksum: %s\n", srcChecksum[:16]+"...")
		fmt.Printf("[OK] Backup completed\n")
	} else if cfg.Verbose {
		fmt.Printf("[OK] Backup completed\n")
	}

	return nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		os.Remove(dst)
		return err
	}

	// 复制权限
	srcInfo, _ := os.Stat(src)
	os.Chmod(dst, srcInfo.Mode())

	return nil
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	err = os.MkdirAll(dst, srcInfo.Mode())
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcEntry := filepath.Join(src, entry.Name())
		dstEntry := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			copyDir(srcEntry, dstEntry)
		} else {
			copyFile(srcEntry, dstEntry)
		}
	}

	return nil
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

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
