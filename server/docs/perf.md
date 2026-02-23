# Server Performance Baseline

本基线针对 `POST /v1/submit` 的真实处理路径执行并发压测，覆盖以下链路：

- 下游 SigV4 验签（`internal/middleware/sigv4`）
- submit handler（含幂等逻辑）
- 审计写入（downstream/upstream/audit_events）
- 上游签名客户端调用（`internal/relay/upstream`，含重试参数）

压测工具为仓库内置脚本：`server/perf/baseline/main.go`（仅 Go 标准库 + 项目现有依赖）。

## 运行环境

- 时间：2026-02-23T21:00:04Z
- 机器：darwin/arm64，12 CPU，`GOMAXPROCS=12`
- Go：`go1.26.0`
- DB：SQLite 临时文件（脚本内部自动创建）
- 上游：脚本内置 fake upstream，固定延迟 `20ms`

## 复现实验命令

```bash
cd server
go run ./perf/baseline -duration 20s -out perf/baseline/latest.json
```

可调参数（默认值）：

- `-low`（8）：低并发场景
- `-high`（48）：高并发场景
- `-duration`（25s）：每个场景持续时间
- `-timeout`（3s）：压测客户端及 relay->upstream 超时
- `-max-retries`（2）：上游重试上限
- `-client-max-idle-per-host`（256）：压测客户端连接池
- `-upstream-delay`（20ms）：fake upstream 固定延迟

## 基线结果

同一轮执行中，两个并发档位结果如下：

| 场景 | 并发 | req/s | P95 latency | error rate |
|---|---:|---:|---:|---:|
| low | 8 | 281.54 | 35.41 ms | 0.000% |
| high | 48 | 687.02 | 113.19 ms | 0.000% |

原始结果文件：`server/perf/baseline/latest.json`

## 参数建议（连接池 / 超时 / 重试）

基于本轮数据与当前代码行为（submit 路径含审计+幂等写入，SQLite 单连接），建议如下：

1. 连接池
   - SQLite（本地开发）：保持当前 `MaxOpenConns=1`（已在 `internal/repository/sqlite/sqlite.go` 固化），避免写锁争用放大。
   - PostgreSQL（生产建议）：在 `DATABASE_URL` 中显式配置 `pool_max_conns`，建议起步 `16~32`，并按 CPU 与实例规格压测调整；高并发下建议至少 `pool_max_conns >= 峰值并发 / 2` 作为初始值。
   - 若使用 pgx 参数，建议同时设置 `pool_min_conns=4~8` 以减少冷启动抖动。

2. 超时
   - `VOLC_TIMEOUT` 当前默认 `30s` 偏保守。若主打交互式 API，建议收敛到 `3s~8s`，避免上游抖动导致连接长期占用。
   - 压测脚本在 `timeout=3s` 下无错误，说明本地路径余量充足；线上应以真实上游 RTT 重新校准。

3. 重试上限
   - 当前上游默认重试 2 次（总尝试 3 次）可提升成功率，但会放大尾延迟。
   - 建议：
     - 低延迟优先业务：`max-retries=1`
     - 成功率优先业务：保留 `max-retries=2`
   - 不建议超过 2，避免在上游故障窗口内形成重试放大。

## 结论

- 在本地可复现环境下，submit 真实链路在 `8 -> 48` 并发提升时吞吐从 `281.54 req/s` 提升到 `687.02 req/s`，P95 从 `35.41ms` 升至 `113.19ms`。
- 两档并发均无错误（`error_rate=0`），可作为后续优化（连接池、超时、重试策略）对比基线。
