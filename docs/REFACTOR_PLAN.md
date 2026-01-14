# MessageBoy 改造方案文档

> 版本: v2.0
> 日期: 2026-01-14
> 状态: 设计阶段

---

## 目录

1. [概述](#1-概述)
2. [整体架构](#2-整体架构)
3. [项目目录结构](#3-项目目录结构)
4. [核心数据模型](#4-核心数据模型)
5. [JSON-RPC 接口设计](#5-json-rpc-接口设计)
6. [通信协议设计](#6-通信协议设计)
7. [WebSocket 加密隧道设计](#7-websocket-加密隧道设计)
8. [配置文件格式](#8-配置文件格式)
9. [安装命令生成](#9-安装命令生成)
10. [实现阶段规划](#10-实现阶段规划)

---

## 1. 概述

### 1.1 背景

当前 MessageBoy 是一个简单的 TCP 转发工具，通过配置文件驱动，功能单一。为满足更复杂的网络转发场景，需要将其改造为支持集中管理的分布式转发系统。

### 1.2 改造目标

- **Server Manager**: 中心化管理服务，提供 JSON-RPC API
- **Client**: 部署在各服务器上的转发代理
- **支持直接转发**: Client 直接转发到目标服务
- **支持中继转发**: 流量经过多个 Client 中继后到达目标

### 1.3 核心特性

| 特性 | 描述 |
|------|------|
| 集中管理 | 通过 Server Manager 统一管理所有 Client |
| 一键部署 | 生成安装命令，在目标服务器执行即可注册 |
| 直接转发 | TCP 流量直接转发到目标地址 |
| 中继转发 | 流量通过 WebSocket 隧道经多个节点中继 |
| 加密传输 | 中继流量使用 AES-256-GCM 对称加密 |

---

## 2. 整体架构

### 2.1 架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Server Manager                               │
│                                                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────────────────┐  │
│  │   JSON-RPC   │  │    Client    │  │      Forward Rule         │  │
│  │    Server    │  │   Registry   │  │        Manager            │  │
│  │   (Gin)      │  │              │  │                           │  │
│  └──────────────┘  └──────────────┘  └───────────────────────────┘  │
│          │                │                       │                  │
│          └────────────────┼───────────────────────┘                  │
│                           │                                          │
│                    ┌──────┴──────┐                                   │
│                    │   Storage   │                                   │
│                    │ (PostgreSQL │                                   │
│                    │   + GORM)   │                                   │
│                    └─────────────┘                                   │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                │ WebSocket + JSON-RPC
                                │
           ┌────────────────────┼────────────────────┐
           │                    │                    │
           ▼                    ▼                    ▼
    ┌────────────┐       ┌────────────┐       ┌────────────┐
    │  Client A  │       │  Client B  │       │  Client C  │
    │            │       │            │       │            │
    │ ┌────────┐ │       │ ┌────────┐ │       │ ┌────────┐ │
    │ │Forwar- │ │       │ │Forwar- │ │       │ │Forwar- │ │
    │ │  der   │ │       │ │  der   │ │       │ │  der   │ │
    │ └────────┘ │       │ └────────┘ │       │ └────────┘ │
    │ ┌────────┐ │       │ ┌────────┐ │       │ ┌────────┐ │
    │ │ Relay  │ │       │ │ Relay  │ │       │ │ Relay  │ │
    │ │ Tunnel │ │       │ │ Tunnel │ │       │ │ Tunnel │ │
    │ └────────┘ │       │ └────────┘ │       │ └────────┘ │
    └────────────┘       └────────────┘       └────────────┘
           │                    │                    │
           ▼                    ▼                    ▼
    ┌────────────┐       ┌────────────┐       ┌────────────┐
    │  Target    │       │  Target    │       │  Target    │
    │  Service   │       │  Service   │       │  Service   │
    └────────────┘       └────────────┘       └────────────┘
```

### 2.2 组件说明

| 组件 | 职责 |
|------|------|
| **Server Manager** | 中心管理服务，提供 API、管理 Client 和转发规则 |
| **Client Registry** | 管理 Client 的注册、心跳、状态 |
| **Forward Rule Manager** | 管理转发规则的 CRUD 和下发 |
| **Storage** | 数据持久化层，使用 PostgreSQL + GORM |
| **Client** | 部署在服务器上的代理，执行实际的转发任务 |
| **Forwarder** | Client 内的转发模块，处理 TCP 流量 |
| **Relay Tunnel** | Client 内的中继模块，处理加密 WebSocket 隧道 |

---

## 3. 项目目录结构

```
MessageBoy/
├── cmd/
│   ├── server/
│   │   └── main.go                    # Server Manager 入口
│   └── client/
│       └── main.go                    # Client 入口
│
├── internal/
│   ├── api/                           # JSON-RPC API 层
│   │   ├── api.go                     # API 服务器初始化
│   │   ├── router.go                  # HTTP 路由定义
│   │   ├── rpc_handler.go             # RPC 请求分发器
│   │   ├── rpc_methods.go             # 基础 RPC 方法 (ping, login)
│   │   ├── client_rpc.go              # Client 管理相关 RPC
│   │   └── forward_rpc.go             # 转发规则相关 RPC
│   │
│   ├── storage/                       # 数据存储层
│   │   ├── storage.go                 # 存储初始化
│   │   ├── client_repo.go             # Client 数据操作
│   │   └── forward_repo.go            # 转发规则数据操作
│   │
│   ├── client/                        # Client 核心逻辑
│   │   ├── client.go                  # Client 主逻辑
│   │   ├── register.go                # 注册到 Server
│   │   ├── heartbeat.go               # 心跳保活
│   │   └── forwarder.go               # TCP 转发处理
│   │
│   ├── relay/                         # 中继模块
│   │   ├── ws_server.go               # WebSocket 服务端 (Server Manager)
│   │   ├── ws_client.go               # WebSocket 客户端 (Client)
│   │   ├── crypto.go                  # 对称加密 (AES-256-GCM)
│   │   └── tunnel.go                  # 隧道管理
│   │
│   ├── conf/                          # 配置管理
│   │   └── config.go                  # 配置加载
│   │
│   └── middleware/                    # HTTP 中间件
│       └── auth.go                    # JWT 认证
│
├── pkg/
│   ├── model/                         # 数据模型
│   │   ├── client.go                  # Client 模型
│   │   └── forward.go                 # 转发规则模型
│   │
│   └── common/
│       ├── resp/                      # JSON-RPC 响应格式
│       │   └── resp.go
│       └── crypto/                    # 加密工具
│           └── aes.go
│
├── configs/
│   ├── server.toml                    # Server 配置文件示例
│   └── client.toml                    # Client 配置文件示例
│
├── scripts/
│   ├── install_server.sh              # Server 安装脚本
│   └── install_client.sh              # Client 安装脚本模板
│
├── web/                               # 前端管理界面 (可选)
│   └── ...
│
├── docs/
│   └── REFACTOR_PLAN.md               # 本文档
│
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 4. 核心数据模型

### 4.1 Client 模型

```go
// pkg/model/client.go
package model

import "time"

// ClientStatus 客户端状态
type ClientStatus string

const (
    ClientStatusOnline  ClientStatus = "online"
    ClientStatusOffline ClientStatus = "offline"
)

// Client 客户端模型
type Client struct {
    // 基础信息
    ID          string       `json:"id" gorm:"primaryKey;type:varchar(36)"`
    Name        string       `json:"name" gorm:"type:varchar(100);not null"`
    Description string       `json:"description" gorm:"type:text"`

    // SSH 信息 (用于远程管理，可选)
    SSHHost     string       `json:"ssh_host" gorm:"type:varchar(255)"`
    SSHPort     int          `json:"ssh_port" gorm:"default:22"`
    SSHUser     string       `json:"ssh_user" gorm:"type:varchar(100)"`
    SSHKeyPath  string       `json:"ssh_key_path" gorm:"type:varchar(500)"`

    // 认证信息
    Token       string       `json:"token" gorm:"type:varchar(64);uniqueIndex"`
    SecretKey   string       `json:"-" gorm:"type:varchar(64)"`  // AES 加密密钥，不暴露给 API

    // 连接状态
    Status      ClientStatus `json:"status" gorm:"type:varchar(20);default:offline"`
    LastIP      string       `json:"last_ip" gorm:"type:varchar(45)"`
    LastSeen    *time.Time   `json:"last_seen"`
    Hostname    string       `json:"hostname" gorm:"type:varchar(255)"`
    Version     string       `json:"version" gorm:"type:varchar(20)"`

    // 元数据
    CreatedAt   time.Time    `json:"created_at"`
    UpdatedAt   time.Time    `json:"updated_at"`
}

// TableName 指定表名
func (Client) TableName() string {
    return "clients"
}
```

### 4.2 转发规则模型

```go
// pkg/model/forward.go
package model

import (
    "database/sql/driver"
    "encoding/json"
    "fmt"
    "time"
)

// ForwardType 转发类型
type ForwardType string

const (
    ForwardTypeDirect ForwardType = "direct"  // 直接转发
    ForwardTypeRelay  ForwardType = "relay"   // 中继转发
)

// StringSlice 用于存储字符串切片到 PostgreSQL JSONB
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
    if s == nil {
        return nil, nil
    }
    return json.Marshal(s)
}

func (s *StringSlice) Scan(value interface{}) error {
    if value == nil {
        *s = nil
        return nil
    }
    bytes, ok := value.([]byte)
    if !ok {
        return fmt.Errorf("failed to scan StringSlice: %v", value)
    }
    return json.Unmarshal(bytes, s)
}

// GormDataType 返回 GORM 数据类型
func (StringSlice) GormDataType() string {
    return "jsonb"
}

// ForwardRule 转发规则模型
type ForwardRule struct {
    // 基础信息
    ID          string      `json:"id" gorm:"primaryKey;type:varchar(36)"`
    Name        string      `json:"name" gorm:"type:varchar(100);not null"`
    Type        ForwardType `json:"type" gorm:"type:varchar(20);not null"`
    Enabled     bool        `json:"enabled" gorm:"default:true"`

    // 入口配置
    ListenAddr   string     `json:"listen_addr" gorm:"type:varchar(100);not null"`   // 监听地址 0.0.0.0:8080
    ListenClient string     `json:"listen_client" gorm:"type:varchar(36);not null"`  // 监听的 Client ID

    // 直接转发配置 (type=direct 时使用)
    TargetAddr   string     `json:"target_addr,omitempty" gorm:"type:varchar(255)"`  // 目标地址 192.168.1.1:80

    // 中继转发配置 (type=relay 时使用)
    RelayChain   StringSlice `json:"relay_chain,omitempty" gorm:"type:jsonb"`        // 中继链 [clientA_id, clientB_id]
    ExitAddr     string      `json:"exit_addr,omitempty" gorm:"type:varchar(255)"`   // 出口地址 192.168.1.1:80

    // 元数据
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName 指定表名
func (ForwardRule) TableName() string {
    return "forward_rules"
}
```

### 4.3 PostgreSQL 数据库初始化

```go
// internal/storage/storage.go
package storage

import (
    "fmt"
    "time"

    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"

    "messageboy/internal/conf"
    "messageboy/pkg/model"
)

type Storage struct {
    DB      *gorm.DB
    Client  *ClientRepository
    Forward *ForwardRepository
}

func NewStorage(cfg *conf.DatabaseConfig) (*Storage, error) {
    dsn := fmt.Sprintf(
        "host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
        cfg.Host,
        cfg.Port,
        cfg.User,
        cfg.Password,
        cfg.DBName,
        cfg.SSLMode,
    )

    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Info),
    })
    if err != nil {
        return nil, fmt.Errorf("failed to connect to database: %w", err)
    }

    // 配置连接池
    sqlDB, err := db.DB()
    if err != nil {
        return nil, fmt.Errorf("failed to get sql.DB: %w", err)
    }
    sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
    sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
    sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

    // 自动迁移
    if err := db.AutoMigrate(
        &model.Client{},
        &model.ForwardRule{},
    ); err != nil {
        return nil, fmt.Errorf("failed to migrate database: %w", err)
    }

    return &Storage{
        DB:      db,
        Client:  NewClientRepository(db),
        Forward: NewForwardRepository(db),
    }, nil
}

func (s *Storage) Close() error {
    sqlDB, err := s.DB.DB()
    if err != nil {
        return err
    }
    return sqlDB.Close()
}
```

### 4.4 数据库 ER 图

```
┌─────────────────────────────────────┐
│              clients                │
├─────────────────────────────────────┤
│ id           VARCHAR(36) PK         │
│ name         VARCHAR(100) NOT NULL  │
│ description  TEXT                   │
│ ssh_host     VARCHAR(255)           │
│ ssh_port     INT DEFAULT 22         │
│ ssh_user     VARCHAR(100)           │
│ ssh_key_path VARCHAR(500)           │
│ token        VARCHAR(64) UNIQUE     │
│ secret_key   VARCHAR(64)            │
│ status       VARCHAR(20)            │
│ last_ip      VARCHAR(45)            │
│ last_seen    TIMESTAMP              │
│ hostname     VARCHAR(255)           │
│ version      VARCHAR(20)            │
│ created_at   TIMESTAMP              │
│ updated_at   TIMESTAMP              │
└─────────────────────────────────────┘
                 │
                 │ 1:N
                 ▼
┌─────────────────────────────────────┐
│          forward_rules              │
├─────────────────────────────────────┤
│ id            VARCHAR(36) PK        │
│ name          VARCHAR(100) NOT NULL │
│ type          VARCHAR(20) NOT NULL  │
│ enabled       BOOLEAN DEFAULT TRUE  │
│ listen_addr   VARCHAR(100) NOT NULL │
│ listen_client VARCHAR(36) FK        │──────┐
│ target_addr   VARCHAR(255)          │      │
│ relay_chain   JSONB                 │      │
│ exit_addr     VARCHAR(255)          │      │
│ created_at    TIMESTAMP             │      │
│ updated_at    TIMESTAMP             │      │
└─────────────────────────────────────┘      │
                                             │
              ┌──────────────────────────────┘
              │ References clients.id
              ▼
```

---

## 5. JSON-RPC 接口设计

### 5.1 接口概览

Server Manager 采用 JSON-RPC 2.0 协议，所有接口通过 `POST /api/rpc` 访问。

#### 请求格式

```json
{
    "jsonrpc": "2.0",
    "id": "request-id",
    "method": "methodName",
    "params": {
        "key": "value"
    }
}
```

#### 成功响应

```json
{
    "jsonrpc": "2.0",
    "id": "request-id",
    "result": {
        "key": "value"
    }
}
```

#### 错误响应

```json
{
    "jsonrpc": "2.0",
    "id": "request-id",
    "error": {
        "code": -32000,
        "message": "error message",
        "data": null
    }
}
```

### 5.2 基础接口

| 方法名 | 认证 | 描述 |
|--------|:----:|------|
| `ping` | 否 | 心跳检测 |
| `adminLogin` | 否 | 管理员登录，返回 JWT Token |

#### ping

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "ping",
    "params": {}
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "pong": true,
        "time": 1705200000,
        "version": "2.0.0"
    }
}
```

#### adminLogin

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "adminLogin",
    "params": {
        "username": "admin",
        "password": "password123"
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "token": "eyJhbGciOiJIUzI1NiIs...",
        "expire_at": "2026-01-15T00:00:00Z"
    }
}
```

### 5.3 Client 管理接口

| 方法名 | 认证 | 描述 |
|--------|:----:|------|
| `createClient` | 是 | 创建 Client |
| `getClientList` | 是 | 获取 Client 列表 |
| `getClient` | 是 | 获取单个 Client |
| `updateClient` | 是 | 更新 Client |
| `deleteClient` | 是 | 删除 Client |
| `regenerateClientToken` | 是 | 重新生成 Token |
| `getClientInstallCommand` | 是 | 获取安装命令 |

#### createClient

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "createClient",
    "params": {
        "name": "HK-Server-01",
        "description": "Hong Kong Server",
        "ssh_host": "192.168.1.100",
        "ssh_port": 22,
        "ssh_user": "root",
        "ssh_key_path": "/root/.ssh/id_rsa"
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "name": "HK-Server-01",
        "token": "abc123def456ghi789",
        "install_command": "curl -sSL https://server.com/install.sh | bash -s -- --server https://server.com --token abc123def456ghi789"
    }
}
```

#### getClientList

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "getClientList",
    "params": {
        "page": 1,
        "limit": 20,
        "search": "HK",
        "status": "online"
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "clients": [
            {
                "id": "550e8400-e29b-41d4-a716-446655440000",
                "name": "HK-Server-01",
                "status": "online",
                "last_ip": "203.0.113.1",
                "last_seen": "2026-01-14T10:00:00Z",
                "created_at": "2026-01-01T00:00:00Z"
            }
        ],
        "total": 1,
        "page": 1,
        "limit": 20,
        "pages": 1
    }
}
```

#### getClient

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "getClient",
    "params": {
        "id": "550e8400-e29b-41d4-a716-446655440000"
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "name": "HK-Server-01",
        "description": "Hong Kong Server",
        "ssh_host": "192.168.1.100",
        "ssh_port": 22,
        "ssh_user": "root",
        "status": "online",
        "last_ip": "203.0.113.1",
        "last_seen": "2026-01-14T10:00:00Z",
        "hostname": "hk-server-01",
        "version": "2.0.0",
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-14T10:00:00Z"
    }
}
```

#### updateClient

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "updateClient",
    "params": {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "name": "HK-Server-01-Updated",
        "description": "Updated description"
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "success": true
    }
}
```

#### deleteClient

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "deleteClient",
    "params": {
        "id": "550e8400-e29b-41d4-a716-446655440000"
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "success": true
    }
}
```

#### regenerateClientToken

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "regenerateClientToken",
    "params": {
        "id": "550e8400-e29b-41d4-a716-446655440000"
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "token": "newtoken123456789",
        "install_command": "curl -sSL https://server.com/install.sh | bash -s -- --server https://server.com --token newtoken123456789"
    }
}
```

#### getClientInstallCommand

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "getClientInstallCommand",
    "params": {
        "id": "550e8400-e29b-41d4-a716-446655440000"
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "install_command": "curl -sSL https://server.com/install.sh | bash -s -- --server https://server.com --token abc123def456ghi789",
        "manual_command": "./messageboy-client --server https://server.com --token abc123def456ghi789"
    }
}
```

### 5.4 转发规则接口

| 方法名 | 认证 | 描述 |
|--------|:----:|------|
| `createForwardRule` | 是 | 创建转发规则 |
| `getForwardRuleList` | 是 | 获取规则列表 |
| `getForwardRule` | 是 | 获取单个规则 |
| `updateForwardRule` | 是 | 更新规则 |
| `deleteForwardRule` | 是 | 删除规则 |
| `toggleForwardRule` | 是 | 启用/禁用规则 |

#### createForwardRule - 直接转发

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "createForwardRule",
    "params": {
        "name": "Web Server Forward",
        "type": "direct",
        "listen_addr": "0.0.0.0:8080",
        "listen_client": "550e8400-e29b-41d4-a716-446655440000",
        "target_addr": "192.168.1.10:80"
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "id": "660e8400-e29b-41d4-a716-446655440001",
        "name": "Web Server Forward",
        "type": "direct"
    }
}
```

#### createForwardRule - 中继转发

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "createForwardRule",
    "params": {
        "name": "Relay to Internal Service",
        "type": "relay",
        "listen_addr": "0.0.0.0:9000",
        "listen_client": "client-a-id",
        "relay_chain": ["client-b-id", "client-c-id"],
        "exit_addr": "192.168.100.50:3306"
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "id": "770e8400-e29b-41d4-a716-446655440002",
        "name": "Relay to Internal Service",
        "type": "relay"
    }
}
```

#### getForwardRuleList

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "getForwardRuleList",
    "params": {
        "page": 1,
        "limit": 20,
        "client_id": "550e8400-e29b-41d4-a716-446655440000",
        "type": "direct",
        "enabled": true
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "rules": [
            {
                "id": "660e8400-e29b-41d4-a716-446655440001",
                "name": "Web Server Forward",
                "type": "direct",
                "enabled": true,
                "listen_addr": "0.0.0.0:8080",
                "listen_client": "550e8400-e29b-41d4-a716-446655440000",
                "listen_client_name": "HK-Server-01",
                "target_addr": "192.168.1.10:80",
                "created_at": "2026-01-01T00:00:00Z"
            }
        ],
        "total": 1,
        "page": 1,
        "limit": 20,
        "pages": 1
    }
}
```

#### toggleForwardRule

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "toggleForwardRule",
    "params": {
        "id": "660e8400-e29b-41d4-a716-446655440001",
        "enabled": false
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "success": true
    }
}
```

### 5.5 Client 内部接口

这些接口供 Client 程序调用，用于注册、心跳和获取规则。

| 方法名 | 认证方式 | 描述 |
|--------|----------|------|
| `clientRegister` | Token | Client 注册 |
| `clientHeartbeat` | SecretKey | 心跳上报 |
| `clientGetRules` | SecretKey | 获取转发规则 |

#### clientRegister

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "clientRegister",
    "params": {
        "token": "abc123def456ghi789",
        "hostname": "hk-server-01",
        "version": "2.0.0"
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "client_id": "550e8400-e29b-41d4-a716-446655440000",
        "secret_key": "base64_encoded_aes_key",
        "ws_endpoint": "wss://server.com/ws",
        "heartbeat_interval": 30
    }
}
```

#### clientHeartbeat

```json
// Request (通过 WebSocket 发送)
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "clientHeartbeat",
    "params": {
        "client_id": "550e8400-e29b-41d4-a716-446655440000",
        "uptime": 3600,
        "connections": 10,
        "bytes_in": 1024000,
        "bytes_out": 2048000
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "ack": true,
        "server_time": 1705200000
    }
}
```

#### clientGetRules

```json
// Request
{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "clientGetRules",
    "params": {
        "client_id": "550e8400-e29b-41d4-a716-446655440000"
    }
}

// Response
{
    "jsonrpc": "2.0",
    "id": "1",
    "result": {
        "rules": [
            {
                "id": "rule-1",
                "type": "direct",
                "listen_addr": "0.0.0.0:8080",
                "target_addr": "192.168.1.10:80"
            },
            {
                "id": "rule-2",
                "type": "relay",
                "listen_addr": "0.0.0.0:9000",
                "relay_chain": ["client-b-id"],
                "exit_addr": "192.168.100.50:3306"
            }
        ],
        "version": "abc123"
    }
}
```

---

## 6. 通信协议设计

### 6.1 Client 注册流程

```
┌────────────────┐                              ┌─────────────────────┐
│     Client     │                              │   Server Manager    │
└───────┬────────┘                              └──────────┬──────────┘
        │                                                  │
        │  1. HTTP POST /api/rpc                           │
        │     method: "clientRegister"                     │
        │     params: {token, hostname, version}           │
        │─────────────────────────────────────────────────>│
        │                                                  │
        │                                    2. 验证 Token │
        │                                    3. 生成 SecretKey
        │                                    4. 更新 Client 状态
        │                                                  │
        │  5. Response: {client_id, secret_key,            │
        │               ws_endpoint, heartbeat_interval}   │
        │<─────────────────────────────────────────────────│
        │                                                  │
        │  6. 建立 WebSocket 连接                           │
        │     URL: wss://server.com/ws?client_id=xxx       │
        │════════════════════════════════════════════════=>│
        │                                                  │
        │  7. 获取转发规则                                  │
        │     method: "clientGetRules"                     │
        │─────────────────────────────────────────────────>│
        │                                                  │
        │  8. 返回规则列表                                  │
        │<─────────────────────────────────────────────────│
        │                                                  │
        │  9. 启动转发服务                                  │
        │                                                  │
        │  10. 心跳 (每 30s)                                │
        │      method: "clientHeartbeat"                   │
        │─────────────────────────────────────────────────>│
        │                                                  │
        │  11. 心跳响应                                     │
        │<─────────────────────────────────────────────────│
        │                                                  │
```

### 6.2 直接转发流程

```
┌────────┐          ┌────────────┐          ┌────────────┐
│  User  │          │   Client   │          │   Target   │
└───┬────┘          └─────┬──────┘          └─────┬──────┘
    │                     │                       │
    │  1. TCP Connect     │                       │
    │     to 0.0.0.0:8080 │                       │
    │────────────────────>│                       │
    │                     │                       │
    │                     │  2. TCP Connect       │
    │                     │     to 192.168.1.10:80│
    │                     │──────────────────────>│
    │                     │                       │
    │                     │  3. Connection OK     │
    │                     │<──────────────────────│
    │                     │                       │
    │  4. Send Data       │                       │
    │────────────────────>│  5. Forward Data     │
    │                     │──────────────────────>│
    │                     │                       │
    │                     │  6. Response Data     │
    │  7. Receive Data    │<──────────────────────│
    │<────────────────────│                       │
    │                     │                       │
```

### 6.3 中继转发流程

```
┌────────┐     ┌────────────┐     ┌────────────┐     ┌────────────┐
│  User  │     │  Client A  │     │  Client B  │     │   Target   │
└───┬────┘     └─────┬──────┘     └─────┬──────┘     └─────┬──────┘
    │                │                  │                  │
    │ 1. TCP Connect │                  │                  │
    │───────────────>│                  │                  │
    │                │                  │                  │
    │                │ 2. WS Tunnel     │                  │
    │                │    Connect Req   │                  │
    │                │    (encrypted)   │                  │
    │                │═════════════════>│                  │
    │                │                  │                  │
    │                │                  │ 3. TCP Connect   │
    │                │                  │─────────────────>│
    │                │                  │                  │
    │                │                  │ 4. Connect OK    │
    │                │ 5. Tunnel ACK    │<─────────────────│
    │                │<═════════════════│                  │
    │                │                  │                  │
    │ 6. Send Data   │                  │                  │
    │───────────────>│ 7. Encrypt +     │                  │
    │                │    Forward       │                  │
    │                │═════════════════>│ 8. Decrypt +    │
    │                │                  │    Forward       │
    │                │                  │─────────────────>│
    │                │                  │                  │
    │                │                  │ 9. Response      │
    │                │ 10. Encrypt +    │<─────────────────│
    │                │     Forward      │                  │
    │ 11. Response   │<═════════════════│                  │
    │<───────────────│                  │                  │
    │                │                  │                  │

═══════ = WebSocket + AES-256-GCM 加密
─────── = 普通 TCP
```

---

## 7. WebSocket 加密隧道设计

### 7.1 隧道消息类型

```go
// internal/relay/tunnel.go
package relay

// 消息类型常量
const (
    MsgTypeConnect  byte = 0x01  // 建立连接请求
    MsgTypeConnAck  byte = 0x02  // 连接确认
    MsgTypeData     byte = 0x03  // 数据传输
    MsgTypeClose    byte = 0x04  // 关闭连接
    MsgTypeError    byte = 0x05  // 错误信息
)

// TunnelMessage 隧道消息结构
type TunnelMessage struct {
    Type     byte   `json:"type"`               // 消息类型
    StreamID uint32 `json:"stream_id"`          // 连接流 ID
    Target   string `json:"target,omitempty"`   // 目标地址 (仅 Connect)
    Payload  []byte `json:"payload,omitempty"`  // 加密后的数据
    Nonce    []byte `json:"nonce,omitempty"`    // AES-GCM nonce
    Error    string `json:"error,omitempty"`    // 错误信息
}
```

### 7.2 消息格式

#### 连接请求 (MsgTypeConnect)

```json
{
    "type": 1,
    "stream_id": 12345,
    "target": "192.168.100.50:3306"
}
```

#### 连接确认 (MsgTypeConnAck)

```json
{
    "type": 2,
    "stream_id": 12345
}
```

#### 数据传输 (MsgTypeData)

```json
{
    "type": 3,
    "stream_id": 12345,
    "payload": "base64_encoded_encrypted_data",
    "nonce": "base64_encoded_nonce"
}
```

#### 关闭连接 (MsgTypeClose)

```json
{
    "type": 4,
    "stream_id": 12345
}
```

#### 错误信息 (MsgTypeError)

```json
{
    "type": 5,
    "stream_id": 12345,
    "error": "connection refused"
}
```

### 7.3 加密方案

使用 AES-256-GCM 对称加密算法：

```go
// pkg/common/crypto/aes.go
package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "io"
)

// Crypto AES-256-GCM 加密器
type Crypto struct {
    key []byte // 32 bytes for AES-256
}

// NewCrypto 创建加密器
func NewCrypto(key []byte) (*Crypto, error) {
    if len(key) != 32 {
        return nil, errors.New("key must be 32 bytes for AES-256")
    }
    return &Crypto{key: key}, nil
}

// GenerateKey 生成随机密钥
func GenerateKey() ([]byte, error) {
    key := make([]byte, 32)
    if _, err := io.ReadFull(rand.Reader, key); err != nil {
        return nil, err
    }
    return key, nil
}

// Encrypt 加密数据
func (c *Crypto) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
    block, err := aes.NewCipher(c.key)
    if err != nil {
        return nil, nil, err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, nil, err
    }

    nonce = make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, nil, err
    }

    ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
    return ciphertext, nonce, nil
}

// Decrypt 解密数据
func (c *Crypto) Decrypt(ciphertext, nonce []byte) (plaintext []byte, err error) {
    block, err := aes.NewCipher(c.key)
    if err != nil {
        return nil, err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    return gcm.Open(nil, nonce, ciphertext, nil)
}

// EncryptToBase64 加密并编码为 Base64
func (c *Crypto) EncryptToBase64(plaintext []byte) (ciphertextB64, nonceB64 string, err error) {
    ciphertext, nonce, err := c.Encrypt(plaintext)
    if err != nil {
        return "", "", err
    }
    return base64.StdEncoding.EncodeToString(ciphertext),
           base64.StdEncoding.EncodeToString(nonce), nil
}

// DecryptFromBase64 从 Base64 解码并解密
func (c *Crypto) DecryptFromBase64(ciphertextB64, nonceB64 string) (plaintext []byte, err error) {
    ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
    if err != nil {
        return nil, err
    }
    nonce, err := base64.StdEncoding.DecodeString(nonceB64)
    if err != nil {
        return nil, err
    }
    return c.Decrypt(ciphertext, nonce)
}
```

### 7.4 隧道建立流程

```
Client A                           Client B
   │                                  │
   │ 1. 用户连接到 Client A 的监听端口  │
   │    (TCP)                         │
   │                                  │
   │ 2. 通过 WebSocket 发送 Connect    │
   │    StreamID: 12345               │
   │    Target: 192.168.100.50:3306   │
   │─────────────────────────────────>│
   │                                  │
   │                                  │ 3. Client B 连接目标
   │                                  │    192.168.100.50:3306
   │                                  │
   │ 4. 返回 ConnAck                   │
   │    StreamID: 12345               │
   │<─────────────────────────────────│
   │                                  │
   │ 5. 双向数据转发                    │
   │    StreamID: 12345               │
   │    Payload: encrypted_data       │
   │<════════════════════════════════>│
   │                                  │
   │ 6. 任意一方关闭                    │
   │    StreamID: 12345               │
   │<─────────────────────────────────│
   │                                  │
```

---

## 8. 配置文件格式

### 8.1 Server 配置 (server.toml)

```toml
# MessageBoy Server Manager 配置文件

[Server]
# 监听地址和端口
Host = "0.0.0.0"
Port = 8080
# 调试模式
Debug = false
# 外部访问地址 (用于生成安装命令)
ExternalURL = "https://messageboy.example.com"

[Database]
# PostgreSQL 配置
Host = "localhost"
Port = 5432
User = "messageboy"
Password = "your-password"
DBName = "messageboy"
SSLMode = "disable"
# 连接池配置
MaxIdleConns = 10
MaxOpenConns = 100
ConnMaxLifetime = 3600  # 秒

[JWT]
# JWT 密钥 (生产环境请更换)
SecretKey = "your-secret-key-change-in-production"
# Token 过期时间 (小时)
ExpireHours = 24
# 签发者
Issuer = "messageboy"

[Admin]
# 管理员账号
Username = "admin"
# 管理员密码 (首次启动后建议通过 API 修改)
Password = "admin123"

[WebSocket]
# WebSocket 端点路径
Endpoint = "/ws"
# 心跳间隔 (秒)
PingInterval = 30
# 心跳超时 (秒)
PongTimeout = 60
# Client 离线判定时间 (秒)
OfflineThreshold = 90

[Logging]
# 日志级别: debug / info / warn / error
Level = "info"
# 日志文件路径 (留空则输出到控制台)
File = ""
# 日志文件最大大小 (MB)
MaxSize = 100
# 日志文件保留天数
MaxAge = 30
```

### 8.2 Client 配置 (client.toml)

```toml
# MessageBoy Client 配置文件

[Client]
# Server Manager 地址
ServerURL = "https://messageboy.example.com"
# 注册令牌
Token = "your-registration-token"

[Connection]
# 重连间隔 (秒)
ReconnectInterval = 5
# 最大重连间隔 (秒)
MaxReconnectInterval = 60
# 心跳间隔 (秒)
HeartbeatInterval = 30

[Logging]
# 日志级别: debug / info / warn / error
Level = "info"
# 日志文件路径 (留空则输出到控制台)
File = ""

[Forwarder]
# 单个连接缓冲区大小 (字节)
BufferSize = 32768
# 连接超时 (秒)
ConnectTimeout = 10
# 空闲超时 (秒)
IdleTimeout = 300
```

---

## 9. 安装命令生成

### 9.1 安装脚本模板

```bash
#!/bin/bash
# MessageBoy Client 安装脚本
set -e

# 参数解析
SERVER_URL=""
TOKEN=""
INSTALL_DIR="/opt/messageboy"

while [[ $# -gt 0 ]]; do
    case $1 in
        --server)
            SERVER_URL="$2"
            shift 2
            ;;
        --token)
            TOKEN="$2"
            shift 2
            ;;
        --dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# 参数验证
if [ -z "$SERVER_URL" ] || [ -z "$TOKEN" ]; then
    echo "Usage: $0 --server <server_url> --token <token> [--dir <install_dir>]"
    exit 1
fi

echo "Installing MessageBoy Client..."
echo "Server: $SERVER_URL"
echo "Install Dir: $INSTALL_DIR"

# 检测系统架构
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        BINARY="messageboy-client-linux-amd64"
        ;;
    aarch64)
        BINARY="messageboy-client-linux-arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# 创建目录
mkdir -p ${INSTALL_DIR}

# 下载二进制文件
echo "Downloading ${BINARY}..."
curl -L -o ${INSTALL_DIR}/messageboy-client "${SERVER_URL}/download/${BINARY}"
chmod +x ${INSTALL_DIR}/messageboy-client

# 生成配置文件
cat > ${INSTALL_DIR}/client.toml << EOF
[Client]
ServerURL = "${SERVER_URL}"
Token = "${TOKEN}"

[Connection]
ReconnectInterval = 5
MaxReconnectInterval = 60
HeartbeatInterval = 30

[Logging]
Level = "info"
File = ""

[Forwarder]
BufferSize = 32768
ConnectTimeout = 10
IdleTimeout = 300
EOF

# 创建 systemd 服务
cat > /etc/systemd/system/messageboy-client.service << 'EOF'
[Unit]
Description=MessageBoy Client
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/messageboy
ExecStart=/opt/messageboy/messageboy-client --config /opt/messageboy/client.toml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 重载 systemd
systemctl daemon-reload

echo ""
echo "Installation completed!"
echo ""
echo "Commands:"
echo "  Start:   systemctl start messageboy-client"
echo "  Enable:  systemctl enable messageboy-client"
echo "  Status:  systemctl status messageboy-client"
echo "  Logs:    journalctl -u messageboy-client -f"
```

### 9.2 生成的安装命令示例

**一键安装 (推荐)**

```bash
curl -sSL https://messageboy.example.com/install.sh | bash -s -- \
  --server https://messageboy.example.com \
  --token abc123-def456-ghi789-jkl012
```

**手动安装**

```bash
# 1. 下载二进制
wget https://messageboy.example.com/download/messageboy-client-linux-amd64 \
  -O /opt/messageboy/messageboy-client
chmod +x /opt/messageboy/messageboy-client

# 2. 运行 (前台模式)
/opt/messageboy/messageboy-client \
  --server https://messageboy.example.com \
  --token abc123-def456-ghi789-jkl012

# 3. 或使用配置文件
/opt/messageboy/messageboy-client --config /opt/messageboy/client.toml
```

---

## 10. 实现阶段规划

### 阶段 1: 基础框架

**目标**: 搭建 Server Manager 基础架构

**任务清单**:
- [ ] 项目目录结构初始化
- [ ] Go 模块配置 (go.mod)
- [ ] 配置文件加载 (Viper + TOML)
- [ ] 日志系统 (Zerolog)
- [ ] 数据库初始化 (GORM + PostgreSQL)
- [ ] HTTP 服务器 (Gin)
- [ ] JSON-RPC 处理框架
- [ ] JWT 认证中间件
- [ ] 基础 RPC 方法 (ping, adminLogin)

**交付物**:
- 可启动的 Server Manager
- `/api/rpc` 端点可用
- ping 和 adminLogin 接口可调用

---

### 阶段 2: Client 管理

**目标**: 实现 Client 的 CRUD 和注册机制

**任务清单**:
- [ ] Client 数据模型和 Repository
- [ ] createClient RPC 方法
- [ ] getClientList RPC 方法
- [ ] getClient RPC 方法
- [ ] updateClient RPC 方法
- [ ] deleteClient RPC 方法
- [ ] regenerateClientToken RPC 方法
- [ ] getClientInstallCommand RPC 方法
- [ ] clientRegister RPC 方法
- [ ] 安装脚本生成

**交付物**:
- Client 管理 API 完整可用
- 可生成安装命令

---

### 阶段 3: Client 实现

**目标**: 实现 Client 程序

**任务清单**:
- [ ] Client 命令行入口
- [ ] 配置文件解析
- [ ] 注册流程实现
- [ ] WebSocket 连接管理
- [ ] 心跳机制
- [ ] 规则获取和更新
- [ ] 编译脚本 (多平台)

**交付物**:
- messageboy-client 可执行文件
- 可注册到 Server Manager
- 心跳保活正常

---

### 阶段 4: 直接转发

**目标**: 实现直接转发功能

**任务清单**:
- [ ] ForwardRule 数据模型和 Repository
- [ ] createForwardRule RPC 方法
- [ ] getForwardRuleList RPC 方法
- [ ] updateForwardRule RPC 方法
- [ ] deleteForwardRule RPC 方法
- [ ] toggleForwardRule RPC 方法
- [ ] clientGetRules RPC 方法
- [ ] Client 端 TCP 转发器实现
- [ ] 规则热更新机制

**交付物**:
- 转发规则管理 API 完整可用
- Client 可执行直接转发
- 支持规则动态更新

---

### 阶段 5: 中继转发

**目标**: 实现 WebSocket 加密隧道中继

**任务清单**:
- [ ] AES-256-GCM 加密模块
- [ ] 隧道消息协议实现
- [ ] WebSocket 服务端 (Server Manager)
- [ ] WebSocket 客户端 (Client)
- [ ] 隧道连接管理
- [ ] 多流复用
- [ ] Client 间中继链路建立
- [ ] 中继转发规则处理

**交付物**:
- 中继转发功能完整可用
- 流量加密传输
- 支持多跳中继

---

### 阶段 6: 完善和优化

**目标**: 功能完善和生产优化

**任务清单**:
- [ ] 连接池优化
- [ ] 错误处理完善
- [ ] 日志和监控
- [ ] 性能调优
- [ ] 单元测试
- [ ] 集成测试
- [ ] 文档完善
- [ ] Docker 镜像构建
- [ ] CI/CD 配置

**交付物**:
- 生产可用版本
- 完整测试覆盖
- Docker 部署支持

---

### 阶段 7: 管理界面 (可选)

**目标**: Web 管理界面

**任务清单**:
- [ ] 前端项目初始化 (React + TypeScript)
- [ ] 登录页面
- [ ] Client 列表页面
- [ ] Client 详情页面
- [ ] 转发规则列表页面
- [ ] 转发规则编辑页面
- [ ] 实时状态监控
- [ ] 前端构建和嵌入

**交付物**:
- Web 管理界面
- 可视化操作

---

## 附录

### A. 错误码定义

| 错误码 | 含义 |
|--------|------|
| -32700 | Parse error - 无效 JSON |
| -32600 | Invalid Request - 无效请求 |
| -32601 | Method not found - 方法不存在 |
| -32602 | Invalid params - 无效参数 |
| -32603 | Internal error - 内部错误 |
| -32000 | Server error - 服务器错误 |
| -32001 | Authentication required - 需要认证 |
| -32002 | Permission denied - 权限不足 |
| -32003 | Resource not found - 资源不存在 |
| -32004 | Resource conflict - 资源冲突 |

### B. 技术栈

| 组件 | 技术选型 |
|------|----------|
| 语言 | Go 1.21+ |
| Web 框架 | Gin |
| ORM | GORM |
| 数据库 | PostgreSQL |
| 配置 | Viper (TOML) |
| 日志 | Zerolog |
| 认证 | JWT (golang-jwt/jwt/v5) |
| WebSocket | gorilla/websocket |
| 加密 | crypto/aes (AES-256-GCM) |
| 前端 | React + TypeScript (可选) |

### C. Docker Compose 示例

```yaml
# docker-compose.yml
version: '3.8'

services:
  postgres:
    image: postgres:16-alpine
    container_name: messageboy-postgres
    environment:
      POSTGRES_USER: messageboy
      POSTGRES_PASSWORD: your-password
      POSTGRES_DB: messageboy
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U messageboy"]
      interval: 10s
      timeout: 5s
      retries: 5

  server:
    build:
      context: .
      dockerfile: Dockerfile.server
    container_name: messageboy-server
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "8080:8080"
    volumes:
      - ./configs/server.toml:/app/configs/server.toml:ro
    environment:
      - DB_HOST=postgres
      - DB_PORT=5432
      - DB_USER=messageboy
      - DB_PASSWORD=your-password
      - DB_NAME=messageboy

volumes:
  postgres_data:
```

### D. 参考资料

- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
- [Gin Web Framework](https://gin-gonic.com/)
- [GORM Documentation](https://gorm.io/)
- [GORM PostgreSQL Driver](https://gorm.io/docs/connecting_to_the_database.html#PostgreSQL)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [WebSocket Protocol](https://tools.ietf.org/html/rfc6455)
- [AES-GCM Encryption](https://en.wikipedia.org/wiki/Galois/Counter_Mode)
