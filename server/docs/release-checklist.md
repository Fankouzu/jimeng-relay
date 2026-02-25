# Jimeng Relay Server 交付清单与回归门禁 (Release Checklist)

本文档汇总了 Jimeng Relay Server 的配置、运行、验证及质量门禁要求，用于确保交付物的可复现性与稳定性。

## 1. 环境配置 (Environment)

| 环境变量 | 默认值 | 说明 | 判定标准 (Pass/Fail) |
| :--- | :--- | :--- | :--- |
| `VOLC_ACCESSKEY` | - | 火山引擎 Access Key (必填) | **Pass**: 服务启动不报错；**Fail**: 启动提示缺失必填项 |
| `VOLC_SECRETKEY` | - | 火山引擎 Secret Key (必填) | **Pass**: 服务启动不报错；**Fail**: 启动提示缺失必填项 |
| `API_KEY_ENCRYPTION_KEY` | - | 32字节 Base64 密钥 (必填) | **Pass**: 可解密 API Key 验签成功；**Fail**: 验签报 401 |
| `DATABASE_TYPE` | `sqlite` | `sqlite` 或 `postgres` | **Pass**: 对应数据库文件生成或连接成功 |
| `DATABASE_URL` | `./jimeng-relay.db` | 数据库连接串 | **Pass**: 自动执行迁移无报错 |
| `SERVER_PORT` | `8080` | 服务监听端口 | **Pass**: `curl localhost:PORT` 有响应 |
| `VOLC_REGION` | `cn-north-1` | 火山引擎 Region | **Pass**: 上游请求 Scope 匹配 |
| `VOLC_TIMEOUT` | `30s` | 上游请求超时 | **Pass**: 慢请求在阈值内返回 |
| `UPSTREAM_MAX_CONCURRENT` | `1` | 上游并发请求上限 | **Pass**: 超出并发进入队列等待 |
| `UPSTREAM_MAX_QUEUE` | `100` | 上游排队队列大小 | **Pass**: 队满立即返回 429 `RATE_LIMITED` |

## 2. 数据库初始化与迁移 (DB Migration)

### SQLite (本地开发)
- **命令**: `DATABASE_TYPE=sqlite DATABASE_URL=./test.db go run cmd/server/main.go`
- **判定标准**:
  - **Pass**: 目录下生成 `test.db`，且 `api_keys`, `audit_events` 等表已创建。
  - **Fail**: 启动日志出现 `migration failed` 或 `database is locked`。

### PostgreSQL (生产环境)
- **命令**: `DATABASE_TYPE=postgres DATABASE_URL=postgres://... go run cmd/server/main.go`
- **判定标准**:
  - **Pass**: `schema_migrations` 表版本对齐最新（当前版本 2）。
  - **Fail**: 提示 `relation "xxx" does not exist` 或连接超时。

## 3. 启动与最小调用步骤 (Startup & Invocation)

1. **启动服务**:
   ```bash
   cd server
   go run cmd/server/main.go
   ```
2. **创建 API Key** (使用 CLI):
   ```bash
   go run cmd/server/main.go key create --description "test-key"
   ```
3. **调用 Submit (SigV4)**:
   使用 CLI 生成的 AK/SK 进行签名调用 `/v1/submit`。
   - **Pass**: 返回上游原始响应（200 或业务错误）。
   - **Fail**: 返回 401 (鉴权失败) 或 500 (审计/系统失败)。

## 4. 安全验证清单 (Security)

| 检查项 | 验证方法 | 判定标准 (Pass/Fail) |
| :--- | :--- | :--- |
| **SigV4 验签** | 使用错误 SK 调用 | **Pass**: 返回 401 `INVALID_SIGNATURE` |
| **Key 吊销** | 使用 `key revoke --id {id}` CLI 命令后请求 | **Pass**: 返回 401 `KEY_REVOKED` |
| **Key 过期** | 修改 DB `expires_at` 为过去时间后请求 | **Pass**: 返回 401 `KEY_EXPIRED` |
| **Scope 约束** | 签名时 Region 传错 (如 `us-east-1`) | **Pass**: 返回 401 `AUTH_FAILED` (Scope mismatch) |
| **敏感脱敏** | 检查 `audit_events` 表 `metadata` | **Pass**: `Authorization`, `sk` 等显示为 `***` |
| **并发限流** | 设置 `UPSTREAM_MAX_CONCURRENT=1`、`UPSTREAM_MAX_QUEUE=2` 后并发 4 个请求 | **Pass**: 第 4 个请求立即返回 429 `RATE_LIMITED` |

## 5. 兼容性验证 (Compatibility)

| 原始路径 (Action 参数) | 转发路径 | 判定标准 (Pass/Fail) |
| :--- | :--- | :--- |
| `POST /?Action=CVSync2AsyncSubmitTask` | `/v1/submit` | **Pass**: 两者行为一致，均能透传 Body |
| `POST /?Action=CVSync2AsyncGetResult` | `/v1/get-result` | **Pass**: 两者行为一致，均能透传 Body |

## 6. 审计与可观测性 (Observability)

- **Fail-Closed 验证**: 模拟数据库断开后发起请求。
  - **判定标准**: **Pass**: 返回 500 `AUDIT_FAILED`；**Fail**: 审计失败但请求成功返回给客户端。
  - **注意**: 审计在请求转发后写入，审计失败会拦截响应返回 500，但不保证上游未被调用。
- **日志追踪**: 检查 stdout 日志。
  - **判定标准**: **Pass**: 每条日志包含 `request_id`，响应日志包含 `latency_ms` 和 `upstream_status`。

## 7. 并发与队列策略验证 (Concurrency & Queueing)

本节验证单 Key 并发限制与全局 FIFO 队列行为。必须提供自动化回归证据。

| 验证项 | 验证方法 | 判定标准 (Pass/Fail) |
| :--- | :--- | :--- |
| **单 Key 并发门禁** | 使用相同 API Key 同时发起 2 个请求 | **Pass**: 第二个请求立即返回 429 `RATE_LIMITED`；**Fail**: 第二个请求进入排队或成功 |
| **全局 FIFO 验证** | 设置 `UPSTREAM_MAX_CONCURRENT=1`，使用不同 Key 发起并发请求 | **Pass**: 请求按到达顺序串行处理；**Fail**: 出现并发调用上游或顺序错乱 |
| **队列溢出验证** | 设置 `UPSTREAM_MAX_QUEUE=1`，并发请求超出 `1(并发)+1(队列)` | **Pass**: 第 3 个请求立即返回 429 `RATE_LIMITED`；**Fail**: 请求被挂起或返回非 429 |
| **回归证据 (Artifact)** | 检查 `server/scripts/artifacts/local_e2e_concurrency.json` | **Pass**: 文件存在且 `ok: true`；**Fail**: 文件缺失或 `ok: false` |

**强制门禁**: 若并发回归证据缺失或 `ok: false`，禁止发布。证据应包含在 `.sisyphus/evidence/` 或脚本默认产物路径中。

## 8. 质量门禁命令 (Quality Gates)

执行以下命令进行回归验证：

```bash
# 1. 单元测试与集成测试 (含 SQLite 内存库验证)
go test -v ./...

# 2. 竞态检测 (并发安全验证)
go test -race ./internal/...

# 3. 静态检查
go vet ./...

# 4. 编译验证
go build -o /dev/null ./cmd/server/main.go
# 5. 本地 E2E 并发回归 (必须通过)
go run scripts/local_e2e_concurrency.go
```

**判定标准**:
- **Pass**: 所有命令退出码为 0，测试全部通过。
- **Fail**: 存在任一 Test Failure 或 Compilation Error。

## 9. 性能基线 (Performance)

- **基线脚本**: `server/perf/baseline/main.go`
- **执行命令**: `go run ./perf/baseline -duration 10s`
- **判定标准**:
  - **Pass**: 吞吐量 (req/s) 不低于 `server/docs/perf.md` 记录值的 80% (环境差异容忍)。
  - **Fail**: 出现非 0 的 `error_rate`。

## 10. 已知限制与后续建议

1. **已知限制**:
   - 仅支持即梦 4.0 异步接口 (`submit`/`get-result`)。
   - 幂等性仅对 `/v1/submit` 生效，兼容路径不启用。
   - 暂无后台定时任务清理过期 API Key 或幂等记录（依赖 `DeleteExpired` 手动触发或后续扩展）。
   - **审计时序限制**: 审计记录在请求转发后写入。若数据库写入失败，服务会向客户端返回 500 `AUDIT_FAILED`，但此时上游任务可能已在处理中。
2. **后续建议**:
   - 生产环境建议开启 PostgreSQL 连接池参数。
   - 建议接入 Prometheus 指标监控（当前仅有结构化日志）。
   - 建议在网关层（如 Nginx/Kong）前置处理 TLS。
