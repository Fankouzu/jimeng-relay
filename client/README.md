# Jimeng CLI Client

`jimeng` 是即梦 AI 图片/视频生成 4.0 的 Go 命令行客户端，支持：

- 文生图（t2i）/ 文生视频（t2v）
- 图生图（i2i）/ 图生视频（i2v）
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
| `VOLC_SCHEME` | 协议 (http/https) | `https` |
| `VOLC_TIMEOUT` | 请求超时 | `30s` |

### 本地测试 Relay Server

如需连接本地 Relay Server 进行测试，请在 `.env` 中配置：

```bash
VOLC_SCHEME=http
VOLC_HOST=localhost:8080
VOLC_TIMEOUT=180s
```

说明：当服务端开启排队/节流时，客户端请求可能会在队列中等待，`VOLC_TIMEOUT` 建议大于服务端最大排队等待时间。

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

## 6. video 命令（视频生成）

### 6.1 视频预设 (Presets)

| 预设名 | 说明 | 对应 ReqKey |
| :--- | :--- | :--- |
| `t2v-720` | 文生视频 720p | `jimeng_t2v_v30_720p` |
| `t2v-1080` | 文生视频 1080p | `jimeng_t2v_v30_1080p` |
| `i2v-first` | 图生视频 (首帧) | `jimeng_i2v_first_v30_1080` |
| `i2v-first-tail` | 图生视频 (首尾帧) | `jimeng_i2v_first_tail_v30_1080` |
| `i2v-recamera` | 图生视频 (运镜) | `jimeng_i2v_recamera_v30` |

### 6.2 视频提交

```bash
# 文生视频
jimeng video submit \
  --preset t2v-720 \
  --prompt "一只在森林中奔跑的小狗" \
  --aspect-ratio 16:9 \
  --format json

# 图生视频
jimeng video submit \
  --preset i2v-first \
  --prompt "让图片中的人物微笑" \
  --image-url "https://example.com/input.png" \
  --format json
```

### 6.3 视频查询、等待与下载

```bash
# 查询状态
jimeng video query --task-id <task_id> --preset t2v-720

# 等待任务完成
jimeng video wait --task-id <task_id> --preset t2v-720 --wait-timeout 10m

# 下载结果
jimeng video download --task-id <task_id> --preset t2v-720 --dir ./outputs
```

下载逻辑说明：

- 任务完成时返回 `video_url`
- `download` 命令会自动下载该 URL 并保存为视频文件（如 `.mp4`）

## 7. 输出文件命名规则

为避免覆盖，下载文件名统一增加任务 ID 前缀：

### 7.1 图片下载 (Image)
- **URL 场景**：`<task_id>-<原始文件名>`
- **Base64 场景**：`<task_id>-image-<序号>.png`

### 7.2 视频下载 (Video)
- **命名规则**：`<task_id>.<扩展名>`
- **扩展名获取**：优先从 `video_url` 中提取扩展名，若无法提取则默认为 `.mp4`。

即使连续多次生成到同一个目录，也不会重名覆盖。


## 8. 常见问题

### 8.1 并发限流 `50430`

当返回类似错误：

`Request Has Reached API Concurrent Limit, Please Try Later`

处理建议：

- 降低 `--count`
- 间隔几秒后重试

### 8.2 任务 done 但无 URL

这是服务端返回形态差异（主要针对图片生成），客户端已支持图片 base64 回退下载，无需手动处理。

## 9. 开发验证命令

```bash
go test ./...
go test -race ./...
go vet ./...
go build -o ./bin/jimeng .
```

## 10. 参考文档

- 火山引擎即梦 AI 图片生成 4.0：
  `https://www.volcengine.com/docs/85621/1817045`
