# 部署指南

本文档介绍如何部署 jimeng-relay 服务端到 Docker 和 Railway。

## 1. 快速开始 (Docker)

### 1.1 使用 Docker Compose (推荐)

```bash
cd server

# 创建 .env 文件
cp .env.example .env
# 编辑 .env 填入必要配置

# 启动服务
docker compose up -d

# 查看日志
docker compose logs -f

# 停止服务
docker compose down
```

### 1.2 使用 Docker 直接运行

```bash
cd server

# 构建镜像
docker build -t jimeng-server .

# 运行容器
docker run -d \
  --name jimeng-server \
  -p 8080:8080 \
  -e VOLC_ACCESSKEY=your_access_key \
  -e VOLC_SECRETKEY=your_secret_key \
  -e API_KEY_ENCRYPTION_KEY=your_base64_key \
  -v jimeng-data:/data \
  jimeng-server

# 检查健康状态
curl http://localhost:8080/health
```

## 2. Railway 部署

### 2.1 创建项目

1. 登录 [Railway](https://railway.app/)
2. 点击 "New Project"
3. 选择 "Deploy from GitHub repo"
4. 选择 `jimeng-relay` 仓库
5. 设置 Root Directory 为 `server`

### 2.2 配置环境变量

在 Railway 项目设置中添加以下环境变量：

| 变量名 | 必填 | 说明 |
|:---|:---:|:---|
| `VOLC_ACCESSKEY` | ✅ | 火山引擎 Access Key |
| `VOLC_SECRETKEY` | ✅ | 火山引擎 Secret Key |
| `API_KEY_ENCRYPTION_KEY` | ✅ | 32字节密钥的 Base64 编码 |
| `DATABASE_TYPE` | | `sqlite` 或 `postgres` |
| `DATABASE_URL` | | 数据库连接字符串 |
| `UPSTREAM_MAX_CONCURRENT` | | 上游并发上限 (默认 1) |
| `UPSTREAM_MAX_QUEUE` | | 排队队列大小 (默认 100) |

**注意**: Railway 会自动注入 `PORT` 环境变量，无需手动设置。

### 2.3 添加 PostgreSQL (推荐生产环境)

1. 在 Railway 项目中点击 "New"
2. 选择 "Database" → "Add PostgreSQL"
3. Railway 会自动注入 `DATABASE_URL` 环境变量
4. 在服务设置中添加变量引用:
   - `DATABASE_TYPE=postgres`
   - `DATABASE_URL=${{Postgres.DATABASE_URL}}`

### 2.4 SQLite 持久化 (开发环境)

如果使用 SQLite，需要添加 Volume：

1. 在服务设置中点击 "Volumes"
2. 添加 Volume，挂载路径为 `/data`
3. 设置 `DATABASE_URL=/data/jimeng-relay.db`

## 3. 健康检查

服务提供两个健康检查端点：

| 端点 | 用途 | 认证 |
|:---|:---|:---|
| `GET /health` | Liveness probe (进程存活) | 不需要 |
| `GET /ready` | Readiness probe (服务就绪) | 不需要 |

响应示例：
```json
// GET /health
{"status": "ok"}

// GET /ready (服务就绪)
{"status": "ok"}

// GET /ready (服务未就绪)
{"status": "error", "message": "database not ready"}
```

## 4. 环境变量说明

### 4.1 必需变量

| 变量名 | 说明 | 示例 |
|:---|:---|:---|
| `VOLC_ACCESSKEY` | 火山引擎 AK | `AKLTM...` |
| `VOLC_SECRETKEY` | 火山引擎 SK | `TkRn...` |
| `API_KEY_ENCRYPTION_KEY` | 加密密钥 | `openssl rand -base64 32` |

### 4.2 可选变量

| 变量名 | 默认值 | 说明 |
|:---|:---|:---|
| `PORT` | `8080` | Railway 自动注入 |
| `SERVER_PORT` | `8080` | 服务监听端口 |
| `DATABASE_TYPE` | `sqlite` | 数据库类型 |
| `DATABASE_URL` | `./jimeng-relay.db` | 数据库连接字符串 |
| `VOLC_HOST` | `visual.volcengineapi.com` | API 地址 |
| `VOLC_REGION` | `cn-north-1` | 区域 |
| `UPSTREAM_MAX_CONCURRENT` | `1` | 上游并发上限 |
| `UPSTREAM_MAX_QUEUE` | `100` | 排队队列大小 |

## 5. API Key 管理

API Key 只能通过 CLI 管理：

```bash
# 构建
docker build -t jimeng-server .

# 创建 Key
docker run --rm \
  -e VOLC_ACCESSKEY=xxx \
  -e VOLC_SECRETKEY=xxx \
  -e API_KEY_ENCRYPTION_KEY=xxx \
  -e DATABASE_URL=/data/jimeng-relay.db \
  -v $(pwd)/data:/data \
  jimeng-server \
  key create --description "prod-client" --expires-at 2026-12-31T23:59:59Z

# 列出 Keys
docker run --rm ... jimeng-server key list

# 吊销 Key
docker run --rm ... jimeng-server key revoke --id key_xxx

# 轮换 Key
docker run --rm ... jimeng-server key rotate --id key_xxx --grace-period 10m
```

## 6. 故障排查

### 6.1 服务无法启动

1. 检查环境变量是否正确设置
2. 查看 Railway 日志
3. 验证 `API_KEY_ENCRYPTION_KEY` 是有效的 Base64 编码

### 6.2 数据库连接失败

1. 检查 `DATABASE_URL` 格式
2. 确认 PostgreSQL 服务已启动 (Railway)
3. 验证网络连接 (私有网络)

### 6.3 健康检查失败

1. 访问 `/health` 端点
2. 检查服务日志
3. 验证端口配置 (`PORT` 环境变量)

### 6.4 请求超时

1. 检查 `VOLC_TIMEOUT` 设置
2. 确认上游服务可用
3. 查看并发配置

## 7. 费用估算 (Railway)

| 资源 | 规格 | 估算费用 |
|:---|:---|:---|
| 服务 | 512MB RAM | ~$5/月 |
| PostgreSQL | 512MB RAM, 10GB | ~$5/月 |
| 网络流量 | 10GB | ~$1/月 |
| **总计** | | **~$11/月** |

费用因实际使用量而异，Railway 按秒计费。
