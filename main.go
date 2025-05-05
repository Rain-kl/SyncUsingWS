package main

import (
	"fmt"
	"log"

	"SyncUsingWS/pkg/client"
	"SyncUsingWS/pkg/config"
	syncPkg "SyncUsingWS/pkg/sync"
)

func main() {
	// 创建默认配置
	cfg := config.NewDefaultConfig()

	// 从命令行参数和配置文件加载配置
	// 如果配置文件不存在，将创建默认配置并退出程序
	cfg.LoadFromArgs()

	// 显示当前模式
	if cfg.Mode == string(config.BackupMode) {
		fmt.Printf("运行模式: 备份 (本地->WebDAV)\n")
	} else {
		fmt.Printf("运行模式: 恢复 (WebDAV->本地)\n")
	}

	if cfg.SyncDelete {
		fmt.Printf("启用删除操作: 目标位置中源位置不存在的文件将被删除\n")
	} else {
		fmt.Printf("未启用删除操作: 仅同步文件，不会删除目标位置的文件\n")
	}

	// 确保本地同步目录存在
	if err := cfg.EnsureLocalDir(); err != nil {
		log.Fatalf("创建本地目录失败: %v", err)
	}

	// 创建WebDAV客户端
	davClient := client.NewWebDAVClient(
		cfg.WebdavURL,
		cfg.WebdavUsername,
		cfg.WebdavPassword,
	)

	// 测试WebDAV连接
	_, err := davClient.FileExists("/")
	if err != nil {
		log.Fatalf("无法连接到WebDAV服务器: %v", err)
	}

	// 创建同步管理器
	syncManager := syncPkg.NewSyncManager(davClient, cfg)

	// 开始同步过程
	if err := syncManager.StartSync(); err != nil {
		log.Fatalf("同步失败: %v", err)
	}
}
