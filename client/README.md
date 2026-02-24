# Jimeng CLI Client

`jimeng` 是即梦 AI 图片生成 4.0 的 Go 命令行客户端，支持：

- 文生图（t2i）
- 图生图（i2i，支持 URL 和本地文件）
- 异步任务查询与等待
- 结果下载（URL 或 base64 回退）

## 1. 安装与构建

在 `client/` 目录下执行：

```bash
go build -o ./bin/jimeng .
./bin/jimeng --help
```

如果希望全局使用：

```bash
cp ./bin/jimeng /usr/local/bin/jimeng
```

## 2. 配置方式

支持三种来源，优先级如下：

1. 命令行 flag（最高）
2. 系统环境变量
3. `.env` 文件（`client/.env`）

### 环境变量

| 变量名 | 说明 | 默认值 |
| --- | --- | --- |
| `VOLC_ACCESSKEY` | 火山引擎 AK（必填） | - |
| `VOLC_SECRETKEY` | 火山引擎 SK（必填） | - |
| `VOLC_REGION` | 区域 | `cn-north-1` |
| `VOLC_HOST` | API Host | `visual.volcengineapi.com` |
| `VOLC_HOST` | API Host | `visual.volcengineapi.com` |
| `VOLC_SCHEME` | 协议 (http/https) | `https` |
| `VOLC_TIMEOUT` | 请求超时 | `30s` |
### 本地测试 Relay Server

如需连接本地 Relay Server 进行测试，请在 `.env` 中配置：```
VOLC_SCHEME=http
VOLC_HOST=localhost:8080
```

### `.env` 使用

```bash
cp .env.example .env
# 编辑 .env 填入你的 AK/SK

set -a
source .env
set +a
```

## 3. submit 命令（核心）

### 3.1 文生图

```bash
jimeng submit \
  --prompt "一张产品海报，简洁背景，高级质感" \
  --resolution 2048x2048 \
  --count 1 \
  --quality 速度优先 \
  --format json
```

### 3.2 图生图（URL 输入）

```bash
jimeng submit \
  --prompt "保留主体，改成未来科技风" \
  --image-url "https://example.com/input.png" \
  --resolution 2048x2048 \
  --quality 质量优先 \
  --format json
```

### 3.3 图生图（本地文件输入）

```bash
jimeng submit \
  --prompt "保留主体，改成未来科技风" \
  --image-file "./input.png" \
  --resolution 2048x2048 \
  --quality 质量优先 \
  --format json
```

### 3.4 一步式提交 + 等待 + 下载

```bash
jimeng submit \
  --prompt "一张产品海报，简洁背景，高级质感" \
  --resolution 2048x2048 \
  --count 3 \
  --quality 质量优先 \
  --wait \
  --wait-timeout 5m \
  --download-dir ./outputs \
  --overwrite \
  --format json
```

## 4. 新增参数说明

- `--resolution <WxH>`：分辨率，默认 `2048x2048`
- `--count <1-4>`：一次生成张数，默认 `1`
- `--quality`：`速度优先|质量优先`（也支持 `speed|quality`），默认 `速度优先`
- `--image-url`：图生图 URL 输入（可重复）
- `--image-file`：图生图本地文件输入（可重复，自动转 base64）

注意：

- `--image-url` 和 `--image-file` 互斥，不能同时使用。
- `--width/--height` 会覆盖 `--resolution`。

## 5. query / wait / download

### query

```bash
jimeng query --task-id <task_id> --format json
```

### wait

```bash
jimeng wait --task-id <task_id> --interval 2s --wait-timeout 5m --format json
```

### download

```bash
jimeng download --task-id <task_id> --dir ./outputs --overwrite --format json
```

下载逻辑说明：

- 优先使用 `image_urls` 下载
- 若无 URL，自动回退到 `binary_data_base64` 解码

## 6. 输出文件命名规则

为避免覆盖，下载文件名带任务 ID 前缀：

- URL 场景：`<task_id>-<原始文件名>`
- base64 场景：`<task_id>-image-1.png`、`<task_id>-image-2.png`

即使连续多次生成到同一个目录，也不会重名覆盖。

## 7. 常见问题

### 7.1 并发限流 `50430`

当返回类似错误：

`Request Has Reached API Concurrent Limit, Please Try Later`

处理建议：

- 降低 `--count`
- 间隔几秒后重试

### 7.2 任务 done 但无 URL

这是服务端返回形态差异，客户端已支持 base64 回退下载，无需手动处理。

## 8. 开发验证命令

```bash
go test ./...
go test -race ./...
go vet ./...
go build -o ./bin/jimeng .
```

## 9. 参考文档

- 火山引擎即梦 AI 图片生成 4.0：
  `https://www.volcengine.com/docs/85621/1817045`
