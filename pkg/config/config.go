package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// SyncMode 定义同步模式
type SyncMode string

const (
	// BackupMode 备份模式：从本地同步到WebDAV
	BackupMode SyncMode = "backup"
	// RestoreMode 恢复模式：从WebDAV同步到本地
	RestoreMode SyncMode = "restore"
)

// Config 存储应用程序配置
type Config struct {
	// WebDAV服务器配置
	WebdavURL      string `toml:"webdav_url"`
	WebdavUsername string `toml:"webdav_username"`
	WebdavPassword string `toml:"webdav_password"`

	// 本地同步配置
	LocalDir string `toml:"local_dir"`

	// 同步模式设置
	Mode           string `toml:"mode"`            // 同步模式: backup (本地->WebDAV) 或 restore (WebDAV->本地)
	SyncDelete     bool   `toml:"sync_delete"`     // 是否删除目标位置中源位置不存在的文件/目录
	CompareContent bool   `toml:"compare_content"` // 是否比较文件内容而不仅仅是时间戳

	// 并发和重试设置
	MaxConcurrent int           `toml:"max_concurrent"`
	MaxRetries    int           `toml:"max_retries"`
	RetryDelay    time.Duration `toml:"retry_delay"`
}

// 默认配置文件名
const (
	DefaultConfigFile = "config.toml"
)

// NewDefaultConfig 返回默认配置
func NewDefaultConfig() *Config {
	return &Config{
		WebdavURL:      "http://localhost:5244/dav",
		WebdavUsername: "guest",
		WebdavPassword: "guest",
		LocalDir:       "./sync",
		Mode:           string(RestoreMode), // 默认为恢复模式（从WebDAV到本地）
		SyncDelete:     false,               // 默认不删除文件
		CompareContent: false,               // 默认只比较修改时间
		MaxConcurrent:  5,
		MaxRetries:     3,
		RetryDelay:     2 * time.Second,
	}
}

// LoadFromArgs 从命令行参数加载配置
func (c *Config) LoadFromArgs() *Config {
	// 解析命令行参数
	configFile := flag.String("config", DefaultConfigFile, "配置文件路径")
	mode := flag.String("mode", "", "同步模式: backup (本地->WebDAV) 或 restore (WebDAV->本地)")
	syncDelete := flag.Bool("sync-delete", false, "是否删除目标位置中源位置不存在的文件/目录")
	flag.Parse()

	// 尝试加载配置文件
	if err := c.LoadFromFile(*configFile); err != nil {
		// 如果配置文件不存在，创建并保存默认配置
		if os.IsNotExist(err) {
			fmt.Printf("配置文件 %s 不存在，创建默认配置文件\n", *configFile)
			if err := c.SaveToFile(*configFile); err != nil {
				fmt.Printf("创建配置文件失败: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("已创建默认配置文件 %s，请根据需要修改配置后重新运行程序\n", *configFile)
			os.Exit(1)
		} else {
			fmt.Printf("加载配置文件失败: %v\n", err)
			os.Exit(1)
		}
	}

	// 命令行参数优先级高于配置文件
	if *mode != "" {
		c.Mode = *mode
	}

	if *syncDelete {
		c.SyncDelete = true
	}

	// 验证模式是否有效
	if c.Mode != string(BackupMode) && c.Mode != string(RestoreMode) {
		fmt.Printf("无效的同步模式: %s, 使用默认的恢复模式\n", c.Mode)
		c.Mode = string(RestoreMode)
	}

	return c
}

// LoadFromFile 从配置文件加载配置
func (c *Config) LoadFromFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	err = toml.Unmarshal(data, c)
	if err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	return nil
}

// SaveToFile 将配置保存到文件
func (c *Config) SaveToFile(filePath string) error {
	// 确保配置文件所在目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建配置文件目录失败: %v", err)
	}

	// 编码配置为TOML
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	return nil
}

// EnsureLocalDir 确保本地同步目录存在
func (c *Config) EnsureLocalDir() error {
	return os.MkdirAll(c.LocalDir, 0755)
}

// GetSyncMode 获取当前同步模式
func (c *Config) GetSyncMode() SyncMode {
	switch c.Mode {
	case string(BackupMode):
		return BackupMode
	case string(RestoreMode):
		return RestoreMode
	default:
		return RestoreMode
	}
}
