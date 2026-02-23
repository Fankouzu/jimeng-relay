# Jimeng Relay Server

Jimeng Relay Server 是一个高性能的即梦 4.0 API 中继服务，旨在为客户端提供统一的鉴权、审计、幂等性支持以及对上游即梦 API 的透明转发。

## 能力边界

- **核心功能**：支持即梦 4.0 的任务提交 (`submit`) 和结果获取 (`get-result`)。
- **鉴权机制**：采用 AWS SigV4 签名算法进行客户端鉴权。
- **审计与监控**：记录所有下游请求与上游尝试，包含延迟、状态码及错误分类。
- **幂等性**：针对 `submit` 接口提供基于 `Idempotency-Key` 的幂等支持。
- **安全设计**：敏感字段（如 API Key Secret）在数据库中加密存储，审计失败采取 Fail-Closed 策略。

## 配置说明

服务通过环境变量或 `.env` 文件进行配置。

| 环境变量 | 必填 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `VOLC_ACCESSKEY` | 是 | - | 火山引擎 Access Key |
| `VOLC_SECRETKEY` | 是 | - | 火山引擎 Secret Key |
| `VOLC_REGION` | 否 | `cn-north-1` | 火山引擎 Region |
| `VOLC_HOST` | 否 | `visual.volcengineapi.com` | 即梦 API 域名 |
| `API_KEY_ENCRYPTION_KEY` | 是 | - | 用于加密 API Key Secret 的 Base64 编码密钥 (32字节) |
| `SERVER_PORT` | 否 | `8080` | 服务监听端口 |
| `DATABASE_TYPE` | 否 | `sqlite` | 数据库类型 (`sqlite` 或 `postgres`) |
| `DATABASE_URL` | 否 | `./jimeng-relay.db` | 数据库连接字符串 |
| `VOLC_TIMEOUT` | 否 | `30s` | 上游请求超时时间 |

> **注意**：`API_KEY_ENCRYPTION_KEY` 必须是 32 字节原始密钥的 Base64 编码字符串。可以使用以下命令生成：
> `openssl rand -base64 32`

## 快速启动 (本地 SQLite)

1. **准备环境**：确保已安装 Go 1.25.0。
2. **配置变量**：在 `server/` 目录下创建 `.env` 文件并填写必要配置。
3. **启动服务**：
   ```bash
   cd server
   go run cmd/server/main.go
   ```
   服务启动后会自动创建 SQLite 数据库文件并执行迁移。

## 线上部署 (PostgreSQL)

1. **数据库准备**：准备一个 PostgreSQL 实例。
2. **设置环境变量**：
   ```bash
   DATABASE_TYPE=postgres
   DATABASE_URL=postgres://user:password@localhost:5432/dbname?sslmode=disable
   ```
3. **运行服务**：
   服务在连接到 PostgreSQL 时会自动执行初始化迁移。

## 客户端迁移说明

客户端从直接调用即梦 API 迁移到使用 Relay Server 仅需两步：

1. **切换 Base URL**：将请求域名指向 Relay Server 的地址（如 `http://localhost:8080`）。
2. **更新 AK/SK**：使用在 Relay Server 管理接口生成的 API Key 对请求进行 SigV4 签名。
   - **Service**: `cv`
   - **Region**: 与服务端配置的 `VOLC_REGION` 一致

### 接口映射

| 功能 | Relay 路径 | 兼容路径 (Action 参数) |
| :--- | :--- | :--- |
| 提交任务 | `/v1/submit` | `/?Action=CVSync2AsyncSubmitTask` |
| 获取结果 | `/v1/get-result` | `/?Action=CVSync2AsyncGetResult` |

## 管理接口 (API Key 管理)

Relay Server 提供了内置的 API Key 管理接口：

- `POST /v1/keys`：创建新的 API Key
- `GET /v1/keys`：列出所有 API Key
- `POST /v1/keys/{id}/revoke`：吊销 API Key
- `POST /v1/keys/{id}/rotate`：轮换 API Key

## 开发与验证

```bash
# 运行所有测试
go test ./...

# 运行竞态检测
go test -race ./...

# 代码检查
go vet ./...

# 编译二进制文件
go build -o jimeng-server ./cmd/server/main.go
```

## 安全与审计

- **脱敏**：日志和审计记录中会自动脱敏敏感字段。
- **Fail-Closed**：如果审计日志记录失败，服务将拒绝处理该请求并返回 500 错误，以确保合规性。
- **未覆盖能力**：当前版本仅支持即梦 4.0 的异步任务提交与查询，暂不支持同步接口或其他火山引擎服务。
