package sync

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"SyncUsingWebDav/pkg/client"
	"SyncUsingWebDav/pkg/config"
	"SyncUsingWebDav/pkg/util"
)

// SyncManager 同步管理器，负责协调同步过程
type SyncManager struct {
	client    *client.WebDAVClient
	config    *config.Config
	semaphore chan struct{} // 用于控制并发
}

// NewSyncManager 创建一个新的同步管理器
func NewSyncManager(client *client.WebDAVClient, cfg *config.Config) *SyncManager {
	return &SyncManager{
		client:    client,
		config:    cfg,
		semaphore: make(chan struct{}, cfg.MaxConcurrent),
	}
}

// StartSync 开始同步过程
func (s *SyncManager) StartSync() error {
	// 根据同步模式执行不同的同步方向
	switch s.config.GetSyncMode() {
	case config.BackupMode:
		log.Printf("运行备份模式: 从本地目录(%s)同步到WebDAV(%s)...", s.config.LocalDir, s.config.WebdavURL)
		return s.BackupToWebDAV()
	case config.RestoreMode:
		log.Printf("运行恢复模式: 从WebDAV(%s)同步到本地目录(%s)...", s.config.WebdavURL, s.config.LocalDir)
		return s.RestoreFromWebDAV()
	default:
		return fmt.Errorf("未知的同步模式: %s", s.config.Mode)
	}
}

// RestoreFromWebDAV 从WebDAV恢复到本地（原有的同步功能）
func (s *SyncManager) RestoreFromWebDAV() error {
	startTime := time.Now()

	// 获取远程文件列表
	remoteFiles, err := s.buildRemoteFileList("/")
	if err != nil {
		return fmt.Errorf("获取WebDAV文件列表失败: %v", err)
	}

	// 同步WebDAV到本地
	err = s.SyncDirectory("/")

	// 如果配置了删除操作，删除本地多余的文件
	if s.config.SyncDelete && err == nil {
		log.Println("检查并删除本地多余的文件...")

		// 获取本地文件列表
		localFiles, err := s.buildLocalFileList()
		if err != nil {
			return fmt.Errorf("获取本地文件列表失败: %v", err)
		}

		// 找出本地多余的文件
		filesToDelete := findExtraFiles(localFiles, remoteFiles)

		// 按照路径长度降序排序，确保先删除子文件和子目录
		sort.Slice(filesToDelete, func(i, j int) bool {
			return len(filesToDelete[i]) > len(filesToDelete[j])
		})

		// 删除多余的文件
		for _, filePath := range filesToDelete {
			localPath := filepath.Join(s.config.LocalDir, filePath)
			log.Printf("删除本地多余文件: %s", localPath)
			if err := os.RemoveAll(localPath); err != nil {
				log.Printf("警告: 删除文件失败: %s: %v", localPath, err)
			}
		}
	}

	elapsed := time.Since(startTime)
	if err != nil {
		log.Printf("恢复失败: %v, 耗时: %s", err, elapsed)
		return err
	}

	log.Printf("恢复完成! 耗时: %s", elapsed)
	return nil
}

// BackupToWebDAV 备份本地文件到WebDAV
func (s *SyncManager) BackupToWebDAV() error {
	startTime := time.Now()

	// 获取本地文件列表
	localFiles, err := s.buildLocalFileList()
	if err != nil {
		return fmt.Errorf("获取本地文件列表失败: %v", err)
	}

	// 同步本地文件到WebDAV
	err = s.syncLocalToWebDAV("/")

	// 如果配置了删除操作，删除远程多余的文件
	if s.config.SyncDelete && err == nil {
		log.Println("检查并删除WebDAV多余的文件...")

		// 获取远程文件列表
		remoteFiles, err := s.buildRemoteFileList("/")
		if err != nil {
			return fmt.Errorf("获取WebDAV文件列表失败: %v", err)
		}

		// 找出远程多余的文件
		filesToDelete := findExtraFiles(remoteFiles, localFiles)

		// 按照路径长度降序排序，确保先删除子文件和子目录
		sort.Slice(filesToDelete, func(i, j int) bool {
			return len(filesToDelete[i]) > len(filesToDelete[j])
		})

		// 删除多余的文件
		for _, filePath := range filesToDelete {
			log.Printf("删除WebDAV多余文件: %s", filePath)
			if err := s.client.RemoveRemote(filePath); err != nil {
				log.Printf("警告: 删除文件失败: %s: %v", filePath, err)
			}
		}
	}

	elapsed := time.Since(startTime)
	if err != nil {
		log.Printf("备份失败: %v, 耗时: %s", err)
		return err
	}

	log.Printf("备份完成! 耗时: %s", elapsed)
	return nil
}

// SyncDirectory 同步指定远程目录到本地
func (s *SyncManager) SyncDirectory(remotePath string) error {
	remoteFiles, err := s.client.ListFiles(remotePath)
	if err != nil {
		return fmt.Errorf("列出远程文件失败: %v", err)
	}

	// 对文件进行排序处理，先分类再排序
	var directories []client.FileInfo
	var regularFiles []client.FileInfo

	for _, file := range remoteFiles {
		if file.IsDir {
			directories = append(directories, file)
		} else {
			regularFiles = append(regularFiles, file)
		}
	}

	// 按照文件名字典序排序
	sort.Slice(directories, func(i, j int) bool {
		return directories[i].Path < directories[j].Path
	})
	sort.Slice(regularFiles, func(i, j int) bool {
		return regularFiles[i].Path < regularFiles[j].Path
	})

	// 合并排序后的文件列表，目录优先
	sortedFiles := append(directories, regularFiles...)

	var wg sync.WaitGroup
	errorsCh := make(chan error, len(sortedFiles))

	for _, file := range sortedFiles {
		wg.Add(1)
		go func(file client.FileInfo) {
			defer wg.Done()

			// 获取信号量，控制并发
			s.semaphore <- struct{}{}
			defer func() { <-s.semaphore }()

			if file.IsDir {
				// 处理目录
				localDirPath := filepath.Join(s.config.LocalDir, file.Path)
				if err := os.MkdirAll(localDirPath, 0755); err != nil {
					errorsCh <- fmt.Errorf("创建本地目录 %s 失败: %v", localDirPath, err)
					return
				}

				if err := s.SyncDirectory(file.Path); err != nil {
					errorsCh <- err
				}
			} else {
				// 处理文件
				if err := s.SyncFile(file); err != nil {
					errorsCh <- err
				}
			}
		}(file)
	}

	wg.Wait()
	close(errorsCh)

	// 收集错误
	var syncErrors []error
	for err := range errorsCh {
		syncErrors = append(syncErrors, err)
	}

	if len(syncErrors) > 0 {
		return fmt.Errorf("同步过程中发生%d个错误，第一个错误: %v", len(syncErrors), syncErrors[0])
	}

	return nil
}

// syncLocalToWebDAV 同步本地目录到WebDAV
func (s *SyncManager) syncLocalToWebDAV(relativePath string) error {
	localPath := filepath.Join(s.config.LocalDir, relativePath)

	entries, err := os.ReadDir(localPath)
	if err != nil {
		return fmt.Errorf("读取本地目录失败 %s: %v", localPath, err)
	}

	// 将文件条目分为目录和普通文件
	var directories []os.DirEntry
	var regularFiles []os.DirEntry

	for _, entry := range entries {
		if entry.IsDir() {
			directories = append(directories, entry)
		} else {
			regularFiles = append(regularFiles, entry)
		}
	}

	// 排序，确保有序处理
	sort.Slice(directories, func(i, j int) bool {
		return directories[i].Name() < directories[j].Name()
	})

	sort.Slice(regularFiles, func(i, j int) bool {
		return regularFiles[i].Name() < regularFiles[j].Name()
	})

	// 合并排序后的列表，先处理目录
	sortedEntries := append(directories, regularFiles...)

	var wg sync.WaitGroup
	errorsCh := make(chan error, len(sortedEntries))

	for _, entry := range sortedEntries {
		entryName := entry.Name()
		entryRelPath := filepath.Join(relativePath, entryName)
		remotePath := entryRelPath
		if remotePath == "." {
			remotePath = "/"
		}
		// 处理 Windows 路径分隔符
		remotePath = filepath.ToSlash(remotePath)

		wg.Add(1)
		go func(entry os.DirEntry, entryRelPath, remotePath string) {
			defer wg.Done()

			// 获取信号量，控制并发
			s.semaphore <- struct{}{}
			defer func() { <-s.semaphore }()

			if entry.IsDir() {
				// 处理目录
				exists, err := s.client.FileExists(remotePath)
				if err != nil {
					errorsCh <- fmt.Errorf("检查远程目录失败 %s: %v", remotePath, err)
					return
				}

				if !exists {
					log.Printf("创建远程目录: %s", remotePath)
					if err := s.client.MakeDir(remotePath); err != nil {
						errorsCh <- fmt.Errorf("创建远程目录失败 %s: %v", remotePath, err)
						return
					}
				}

				// 递归处理子目录
				if err := s.syncLocalToWebDAV(entryRelPath); err != nil {
					errorsCh <- err
				}
			} else {
				// 处理文件
				if err := s.syncLocalFileToWebDAV(entryRelPath, remotePath); err != nil {
					errorsCh <- err
				}
			}
		}(entry, entryRelPath, remotePath)
	}

	wg.Wait()
	close(errorsCh)

	// 收集错误
	var syncErrors []error
	for err := range errorsCh {
		syncErrors = append(syncErrors, err)
	}

	if len(syncErrors) > 0 {
		return fmt.Errorf("同步过程中发生%d个错误，第一个错误: %v", len(syncErrors), syncErrors[0])
	}

	return nil
}

// syncLocalFileToWebDAV 同步单个本地文件到WebDAV
func (s *SyncManager) syncLocalFileToWebDAV(relPath, remotePath string) error {
	localPath := filepath.Join(s.config.LocalDir, relPath)

	// 获取本地文件信息
	localInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("获取本地文件信息失败 %s: %v", localPath, err)
	}

	// 检查远程文件是否存在
	needsUpload := true
	exists, err := s.client.FileExists(remotePath)
	if err != nil {
		return fmt.Errorf("检查远程文件失败 %s: %v", remotePath, err)
	}

	if exists {
		// 获取远程文件信息
		remoteFiles, err := s.client.ListFiles(filepath.Dir(remotePath))
		if err != nil {
			return fmt.Errorf("获取远程目录信息失败 %s: %v", filepath.Dir(remotePath), err)
		}

		// 查找匹配的远程文件
		for _, remoteFile := range remoteFiles {
			if filepath.Base(remoteFile.Path) == filepath.Base(remotePath) {
				// 比较修改时间
				localModTime := localInfo.ModTime()
				remoteModTime := remoteFile.LastModified

				// 允许 1 秒的时间差
				if localModTime.Add(time.Second).After(remoteModTime) &&
					localModTime.Add(-time.Second).Before(remoteModTime) {
					log.Printf("跳过未修改的文件: %s", remotePath)
					needsUpload = false
				}
				break
			}
		}
	}

	if needsUpload {
		log.Printf("上传文件: %s (大小: %s)", remotePath, formatSize(localInfo.Size()))

		// 使用重试机制上传文件
		err := util.Retry(s.config.MaxRetries, s.config.RetryDelay, func() error {
			return s.client.UploadFile(localPath, remotePath, localInfo.ModTime())
		})

		if err != nil {
			log.Printf("上传失败: %s: %v", remotePath, err)
			return err
		}

		log.Printf("完成上传: %s (%s)", remotePath, formatSize(localInfo.Size()))
		return nil
	}

	return nil
}

// SyncFile 同步单个文件
func (s *SyncManager) SyncFile(file client.FileInfo) error {
	localPath := filepath.Join(s.config.LocalDir, file.Path)

	// 检查本地文件是否存在
	needsDownload := true
	stat, err := os.Stat(localPath)
	if err == nil {
		// 文件存在，比较修改时间
		localModTime := stat.ModTime()

		// 允许 1 秒的时间差，因为不同系统可能会有微小差异
		if localModTime.Add(time.Second).After(file.LastModified) &&
			localModTime.Add(-time.Second).Before(file.LastModified) {
			log.Printf("跳过未修改的文件: %s", file.Path)
			needsDownload = false
		}
	}

	if needsDownload {
		log.Printf("下载文件: %s (大小: %s)", file.Path, formatSize(file.Size))

		// 确保父目录存在
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %v", filepath.Dir(localPath), err)
		}

		// 使用重试机制下载文件
		err := util.Retry(s.config.MaxRetries, s.config.RetryDelay, func() error {
			return s.client.DownloadFile(file.Path, localPath, file.LastModified)
		})

		if err != nil {
			log.Printf("下载失败: %s: %v", file.Path, err)
			return err
		}

		log.Printf("完成下载: %s (%s)", file.Path, formatSize(file.Size))
		return nil
	}

	return nil
}

// buildLocalFileList 构建本地文件列表（相对路径）
func (s *SyncManager) buildLocalFileList() ([]string, error) {
	var files []string
	baseDir := s.config.LocalDir

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过根目录本身
		if path == baseDir {
			return nil
		}

		// 获取相对路径
		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}

		// 转换为WebDAV风格路径
		relPath = filepath.ToSlash(relPath)
		files = append(files, relPath)

		return nil
	})

	return files, err
}

// buildRemoteFileList 递归构建远程文件列表
func (s *SyncManager) buildRemoteFileList(remotePath string) ([]string, error) {
	var files []string

	// 获取当前目录下的文件
	entries, err := s.client.ListFiles(remotePath)
	if err != nil {
		return nil, err
	}

	// 将当前路径添加到列表中（除了根路径）
	if remotePath != "/" && remotePath != "" {
		files = append(files, remotePath)
	}

	// 处理所有文件
	for _, entry := range entries {
		files = append(files, entry.Path)

		// 如果是目录，递归处理
		if entry.IsDir {
			subFiles, err := s.buildRemoteFileList(entry.Path)
			if err != nil {
				return nil, err
			}
			files = append(files, subFiles...)
		}
	}

	return files, nil
}

// findExtraFiles 找出在source中存在但在target中不存在的文件
func findExtraFiles(source, target []string) []string {
	// 创建target的查找映射
	targetMap := make(map[string]bool)
	for _, file := range target {
		targetMap[file] = true
	}

	// 找出多余的文件
	var extras []string
	for _, file := range source {
		if !targetMap[file] {
			extras = append(extras, file)
		}
	}

	return extras
}

// formatSize 格式化文件大小
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
