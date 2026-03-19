# bak - 文件备份工具

一个轻量级的文件/目录备份工具，支持时间戳命名、批量备份和元数据记录。

## 特性

- 📦 **简单易用** - 用法接近 `cp`，零学习成本
- ⏰ **时间戳备份** - 自动生成带时间戳的备份文件名
- 📂 **目录支持** - 支持文件和目录备份
- 🔒 **原子操作** - 备份失败不影响原文件
- ✅ **校验和验证** - SHA256 校验保证备份完整性
- 📋 **元数据记录** - 记录备份来源、时间、大小等信息

## 安装

### 方法一：直接下载二进制

从 [Releases](https://github.com/YOUR_USERNAME/backup-tool/releases) 下载对应平台的二进制文件。

### 方法二：从源码编译

```bash
git clone https://github.com/YOUR_USERNAME/backup-tool.git
cd backup-tool
go build -o bak .
sudo mv bak /usr/local/bin/
```

## 使用方法

### 基本备份

```bash
# 备份单个文件（备份到同目录的 xxx.bak/ 文件夹）
bak report.pdf

# 备份到指定目录
bak data.csv -d /backup/

# 备份目录
bak /etc/nginx/
```

### 常用选项

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-d, --dest` | 备份目标目录 | `<源文件>.bak/` |
| `-s, --suffix` | 备份后缀 | `.bak` |
| `-f, --time-format` | 时间戳格式 | `20060102_150405` |
| `-n, --dry-run` | 模拟运行 | false |
| `-v, --verbose` | 详细输出 | false |
| `-q, --quiet` | 静默模式 | false |

### 示例

```bash
# 模拟运行（先看效果，不实际操作）
bak important.dat -n

# 自定义后缀
bak config.yaml -s ".backup"

# 自定义时间戳格式
bak file.txt -f "2006-01-02_15-04-05"

# 备份到指定目录
bak file.txt -d /backups/

# 批量备份多个文件
bak file1.txt file2.txt file3.txt

# 详细输出模式
bak -v report.pdf

# 静默模式（无输出）
bak -q data.csv
```

### 时间戳格式

默认格式 `20060102_150405` 生成：`report.bak.20260319_143000.pdf`

常用格式：
- `-f "20060102_150405"` → `20060319_143000`
- `-f "2006-01-02_15-04-05"` → `2026-03-19_14-30-00`
- `-f "2006_01_02"` → `2026_03_19`

## 输出示例

```
✓ backup: report.pdf -> /backup/report.bak/report.bak.20260319_143000.pdf (1.5 MB)
```

详细模式：
```
[INFO] Starting backup...
[SOURCE] report.pdf (1.5 MB, sha256:abc123...)
[DEST]   /backup/report.bak/report.bak.20260319_143000.pdf
[VERIFY] checksum: sha256:abc123...
[OK] Backup completed
```

## 备份结构

```
.
├── report.pdf
└── report.bak/
    ├── .bak.meta.json          # 元数据
    ├── report.bak.20260319_143000.pdf
    ├── report.bak.20260318_103200.pdf
    └── report.bak.20260317_091500.pdf
```

元数据文件 `.bak.meta.json`：
```json
{
  "version": "1.0",
  "source": "report.pdf",
  "source_path": "/home/user/report.pdf",
  "created_at": "2026-03-19T14:30:00Z",
  "size": 1572864,
  "checksum": "sha256:abc123..."
}
```

## 退出码

| 退出码 | 含义 |
|--------|------|
| 0 | 备份成功 |
| 1 | 通用错误（参数错误、源文件不存在等） |
| 2 | 权限问题 |
| 3 | 磁盘空间不足 |
| 4 | checksum 验证失败 |

## 开发

```bash
# 克隆项目
git clone https://github.com/YOUR_USERNAME/backup-tool.git
cd backup-tool

# 运行测试
go run main.go -v test.txt

# 编译
go build -o bak .

# 查看帮助
./bak --help
```

## License

MIT
