package client

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
)

// FileInfo 文件信息结构
type FileInfo struct {
	Path         string
	IsDir        bool
	LastModified time.Time
	Size         int64
}

// WebDAVClient WebDAV客户端封装
type WebDAVClient struct {
	client *gowebdav.Client
}

// NewWebDAVClient 创建新的WebDAV客户端
func NewWebDAVClient(url, username, password string) *WebDAVClient {
	// 创建WebDAV客户端
	davClient := gowebdav.NewClient(url, username, password)

	// 配置客户端
	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     30 * time.Second,
	}
	davClient.SetTransport(transport)

	return &WebDAVClient{
		client: davClient,
	}
}

// ListFiles 列出远程目录中的所有文件
func (c *WebDAVClient) ListFiles(remotePath string) ([]FileInfo, error) {
	files, err := c.listRemoteFiles(remotePath)
	if err != nil {
		return nil, err
	}

	var result []FileInfo
	for _, file := range files {
		// 跳过当前目录和父目录引用
		path := file.Path
		name := filepath.Base(path)
		if name == "." || name == ".." {
			continue
		}
		result = append(result, file)
	}
	return result, nil
}

// listRemoteFiles 列出远程目录中的所有文件的实现
func (c *WebDAVClient) listRemoteFiles(remotePath string) ([]FileInfo, error) {
	// 确保路径以 / 开始
	if !strings.HasPrefix(remotePath, "/") {
		remotePath = "/" + remotePath
	}

	// 处理根路径的特殊情况
	if remotePath == "/" {
		remotePath = ""
	}

	files, err := c.client.ReadDir(remotePath)
	if err != nil {
		return nil, fmt.Errorf("读取目录 %s 失败: %v", remotePath, err)
	}

	var result []FileInfo
	for _, file := range files {
		// 构建完整路径
		path := remotePath
		if path != "" && !strings.HasSuffix(path, "/") {
			path += "/"
		}
		path += file.Name()

		result = append(result, FileInfo{
			Path:         path,
			IsDir:        file.IsDir(),
			LastModified: file.ModTime(),
			Size:         file.Size(),
		})
	}

	return result, nil
}

// ReadStream 获取远程文件的读取流
func (c *WebDAVClient) ReadStream(remotePath string) (io.ReadCloser, error) {
	return c.client.ReadStream(remotePath)
}

// DownloadFile 下载文件到指定本地路径
func (c *WebDAVClient) DownloadFile(remotePath, localPath string, remoteModTime time.Time) error {
	// 先获取文件信息以了解文件大小
	_, err := c.client.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("获取远程文件信息失败: %v", err)
	}

	// 读取远程文件
	reader, err := c.ReadStream(remotePath)
	if err != nil {
		return fmt.Errorf("读取远程文件失败: %v", err)
	}
	defer reader.Close()

	// 创建临时文件
	tmpFile := localPath + ".download"
	file, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	// 确保在函数返回前关闭并删除临时文件（如果出错）
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(tmpFile)
		}
	}()

	// 直接复制文件
	_, err = io.Copy(file, reader)
	if err != nil {
		return err
	}

	// 关闭文件
	if err = file.Close(); err != nil {
		return err
	}

	// 原子性地替换文件
	if err = os.Rename(tmpFile, localPath); err != nil {
		return err
	}

	// 设置文件修改时间与远程文件一致
	return os.Chtimes(localPath, remoteModTime, remoteModTime)
}

// UploadFile 上传本地文件到WebDAV服务器
func (c *WebDAVClient) UploadFile(localPath, remotePath string, localModTime time.Time) error {
	// 获取本地文件信息
	_, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("获取本地文件信息失败: %v", err)
	}

	// 打开本地文件
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开本地文件失败: %v", err)
	}
	defer file.Close()

	// 确保远程目录存在
	remoteDir := filepath.Dir(remotePath)
	if remoteDir != "." && remoteDir != "/" {
		if err := c.MakeDir(remoteDir); err != nil {
			return fmt.Errorf("创建远程目录失败: %v", err)
		}
	}

	// 上传文件
	err = c.client.WriteStream(remotePath, file, 0644)
	if err != nil {
		return fmt.Errorf("上传文件失败: %v", err)
	}

	return nil
}

// MakeDir 在远程创建目录（包括多级目录）
func (c *WebDAVClient) MakeDir(remotePath string) error {
	// 处理路径
	remotePath = strings.TrimPrefix(remotePath, "/")
	if remotePath == "" {
		return nil // 根目录不需要创建
	}

	parts := strings.Split(remotePath, "/")
	current := ""

	// 递归创建目录
	for _, part := range parts {
		if current != "" {
			current += "/"
		}
		current += part

		// 尝试创建目录（如果已存在则忽略错误）
		err := c.client.Mkdir(current, 0755)
		if err != nil {
			// 检查目录是否已存在
			info, statErr := c.client.Stat(current)
			if statErr != nil || !info.IsDir() {
				return fmt.Errorf("创建目录 %s 失败: %v", current, err)
			}
		}
	}

	return nil
}

// RemoveRemote 删除远程文件或目录
func (c *WebDAVClient) RemoveRemote(remotePath string) error {
	return c.client.Remove(remotePath)
}

// RemoveRemoteAll 递归删除远程目录及其内容
func (c *WebDAVClient) RemoveRemoteAll(remotePath string) error {
	// 先检查是否存在
	info, err := c.client.Stat(remotePath)
	if err != nil {
		// 如果路径本身就不存在，视为删除成功
		return nil
	}

	// 如果是目录，先删除其中的内容
	if info.IsDir() {
		files, err := c.ListFiles(remotePath)
		if err != nil {
			return fmt.Errorf("列出远程目录失败: %v", err)
		}

		for _, file := range files {
			if err := c.RemoveRemoteAll(file.Path); err != nil {
				return err
			}
		}
	}

	// 最后删除自身
	return c.client.Remove(remotePath)
}

// FileExists 检查远程文件或目录是否存在
func (c *WebDAVClient) FileExists(remotePath string) (bool, error) {
	_, err := c.client.Stat(remotePath)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
