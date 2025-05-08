# SyncUsingWebDav

SyncUsingWebDAV 是一个基于 Go 语言的 WebDAV 文件同步工具，支持本地目录与 WebDAV 服务器之间的双向同步。

## 功能特点

- **双向同步**：支持备份模式（本地→WebDAV）和恢复模式（WebDAV→本地）
- **增量更新**：根据修改时间自动跳过未修改的文件
- **并行传输**：支持多文件并行上传/下载，提高同步效率
- **实时进度**：显示详细的传输进度、速度和完成百分比
- **自动重试**：遇到网络问题自动重试，可配置重试次数和间隔
- **删除同步**：可选择是否删除目标位置中源位置不存在的文件（镜像同步）

## 安装

### 前提条件

- Go 1.18 或更高版本

### 从源码构建

```bash
# 克隆仓库
git clone https://github.com/Rain-kl/SyncUsingWebDav.git
cd SyncUsingWebDav

# 构建项目
go build
```

## 快速开始

1. 首次运行程序时，会自动生成默认配置文件 `config.toml`：

```bash
./SyncUsingWS
```

2. 编辑配置文件 `config.toml`，设置 WebDAV 服务器地址、账号密码和同步目录等信息

3. 运行同步命令：

```bash
# 使用配置文件中的默认模式
./SyncUsingWS

# 指定同步模式（备份或恢复）
./SyncUsingWS -mode backup
./SyncUsingWS -mode restore

# 启用删除操作（对目标位置进行镜像同步）
./SyncUsingWS -sync-delete

# 指定配置文件路径
./SyncUsingWS -config /path/to/config.toml
```

## 配置说明

配置文件 `config.toml` 参数说明：

```toml
# WebDAV 服务器配置
webdav_url = 'http://localhost:5244/dav'    # WebDAV 服务器地址
webdav_username = 'guest'                   # WebDAV 用户名
webdav_password = 'guest'                   # WebDAV 密码

# 同步配置
local_dir = './sync'                        # 本地同步目录
mode = 'restore'                            # 同步模式: backup (本地->WebDAV) 或 restore (WebDAV->本地)
sync_delete = false                         # 是否删除目标位置中源位置不存在的文件/目录
compare_content = false                     # 是否比较文件内容而不仅仅是时间戳

# 性能和稳定性配置
max_concurrent = 5                          # 最大并发传输数
max_retries = 3                             # 失败重试次数
retry_delay = 2000000000                    # 重试延迟（纳秒，2000000000=2秒）
```

## 项目结构

```
SyncUsingWS/
├── main.go                # 主程序入口
├── config.toml            # 配置文件
├── pkg/                   # 包目录
│   ├── client/            # WebDAV 客户端实现
│   │   └── webdav.go
│   ├── config/            # 配置处理
│   │   └── config.go
│   ├── sync/              # 同步逻辑
│   │   └── sync.go
│   └── util/              # 工具函数
│       └── retry.go       # 重试机制
```

## 兼容性

- 支持标准 WebDAV 服务器，已测试兼容：
  - NextCloud
  - AList
  - 坚果云
  - OwnCloud
  - 等标准 WebDAV 服务

## 常见问题

1. **连接失败**：请检查 WebDAV 服务器地址、用户名和密码是否正确
2. **同步速度慢**：可以适当增加 `max_concurrent` 参数提高并发数
3. **同步错误**：检查网络连接，并尝试增加 `max_retries` 和 `retry_delay`

## 许可证

此项目采用 MIT 许可证
