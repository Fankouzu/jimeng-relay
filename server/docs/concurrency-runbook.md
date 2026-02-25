# Jimeng Relay Concurrency & Incident Runbook

本文档提供了 Jimeng Relay Server 并发控制系统的故障排查指南与应急处理流程。

## 1. 核心机制概览 (Core Mechanism)

Jimeng Relay 采用两层并发控制：
1. **Key 级并发 (Per-Key Concurrency)**: 每个 API Key 同时只能有一个活跃请求。由 `KeyManager` 在内存中维护 `inUse` 状态。
2. **全局并发 (Global Concurrency)**: 限制转发到上游的总并发数。由 `upstream.Client` 使用信号量 (Semaphore) 和等待队列 (Queue) 实现。

## 2. 诊断工具箱 (Diagnostic Toolkit)

### 2.1 观察运行时日志
通过实时日志观察 Key 的获取与释放行为：
```bash
# 观察 Key 获取/释放及并发排队日志
tail -f server.log | grep -E "key acquired|upstream queue"

### 2.2 数据库状态检查
检查 API Key 是否被吊销或过期：
```bash
# SQLite
sqlite3 jimeng-relay.db "SELECT id, description, revoked, expires_at FROM api_keys WHERE id = 'key_id';"

# PostgreSQL
psql $DATABASE_URL -c "SELECT id, description, revoked, expires_at FROM api_keys WHERE id = 'key_id';"
```

### 2.3 并发压力测试 (复现工具)
使用内置脚本模拟高并发场景：
```bash
# 模拟 5 个并发请求，观察排队与限流行为
go run ./scripts/local_e2e_concurrency.go
```

---

## 3. 常见故障场景 (Incident Scenarios)

### 3.1 场景 A: 特定 Key 永久繁忙 (Key Permanently Busy)
**症状**: 客户端持续收到 `429 Too Many Requests`，错误信息为 `api key is already in use`。

**排查步骤**:
1. **检查日志**: 搜索该 `api_key_id` 的日志。
   - 如果只有 `key acquired` 而没有对应的响应日志（包含 `latency_ms`），说明请求可能在上游挂起或发生死锁。
2. **确认上游状态**: 检查火山引擎控制台，确认是否有长时间运行的任务。
3. **验证超时配置**: 检查 `VOLC_TIMEOUT` 是否设置过长。

**修复方案**:
- **临时方案**: 重启 Jimeng Relay 服务。由于 `inUse` 状态存储在内存中，重启将重置所有 Key 的状态。
- **长期方案**: 确保客户端设置了合理的 Request Context Timeout。

---

### 3.2 场景 B: 全局队列阻塞 (Global Queue Stuck)
**症状**: 所有请求均返回 `429 Too Many Requests`，错误信息为 `upstream queue is full`。

**排查步骤**:
1. **检查上游延迟**: 查看日志中的 `latency_ms`。如果延迟普遍接近 `VOLC_TIMEOUT`，说明上游处理能力达到瓶颈。
2. **检查并发配置**:
   ```bash
   echo $UPSTREAM_MAX_CONCURRENT
   echo $UPSTREAM_MAX_QUEUE
   ```
3. **观察 Worker 状态**: 如果 `latency_ms` 正常但队列依然满，可能存在信号量泄露。

**修复方案**:
- **紧急恢复**: 重启服务以清空内存队列和信号量。
- **调优**: 根据上游配额适当调大 `UPSTREAM_MAX_CONCURRENT`。

---

### 3.3 场景 C: 吊销行为异常 (Revoke Anomaly)
**症状**: 使用 `jimeng-server key revoke` 吊销 Key 后，客户端仍能调用成功；或未吊销的 Key 提示 `KEY_REVOKED`。

**排查步骤**:
1. **检查数据库**:
   ```bash
   sqlite3 jimeng-relay.db "SELECT revoked FROM api_keys WHERE id = 'key_xxx';"
   ```
2. **检查签名验证**: 确认客户端是否使用了旧的缓存签名（SigV4 签名通常有 5 分钟有效期，但服务端每请求都会校验 DB）。

**修复方案**:
- 如果数据库状态正确但行为异常，请检查服务是否连接到了正确的数据库实例。
- 确认没有多个服务实例连接到不同的数据库。

---

## 4. 应急命令速查 (Cheat Sheet)

| 目的 | 命令 |
| :--- | :--- |
| **查看当前配置** | `grep -E "UPSTREAM|VOLC" .env` |
| **强制重置状态** | `systemctl restart jimeng-relay` (或对应容器重启命令) |
| **查看实时错误日志** | `tail -f server.log | grep -E "level=ERROR|level=WARN"` |
| **统计错误分布** | `grep "upstream_status" server.log | awk '{print $NF}' | sort | uniq -c` |

## 5. 运维建议
- **监控**: 建议监控 `429` 错误率。如果 `api key is already in use` 占比高，说明客户端并发模型有问题；如果 `upstream queue is full` 占比高，说明服务端并发限制过紧。
- **超时**: 始终设置 `VOLC_TIMEOUT`（默认 30s），防止上游挂起导致信号量耗尽。
