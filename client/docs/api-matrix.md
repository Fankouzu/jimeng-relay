# 即梦 AI 图片/视频生成 4.0 API 矩阵（冻结版）

## 范围与冻结约束

- 覆盖即梦 AI 图片生成 4.0 (`req_key=jimeng_t2i_v40`) 及视频生成 3.0。
- API 动作仅允许：`CVSync2AsyncSubmitTask`、`CVSync2AsyncGetResult`。
- 不纳入 `CVProcess` / `GetTaskResult` 主路径。

## 全局调用约定

- Endpoint: `https://visual.volcengineapi.com`
- Query: `Version=2022-08-31`
- Header 固定语义：`Region=cn-north-1`，`Service=cv`

## 命令与接口映射

| 命令名 | Action | req_key | 说明 |
| --- | --- | --- | --- |
| `jimeng.image.v40.submit` | `CVSync2AsyncSubmitTask` | `jimeng_t2i_v40` | 提交图片生成任务 |
| `jimeng.image.v40.get` | `CVSync2AsyncGetResult` | `jimeng_t2i_v40` | 查询图片任务状态与结果 |
| `jimeng.video.v30.submit` | `CVSync2AsyncSubmitTask` | (见下表) | 提交视频生成任务 |
| `jimeng.video.v30.get` | `CVSync2AsyncGetResult` | (见下表) | 查询视频任务状态与结果 |

## 参数矩阵

### 1) `jimeng.image.v40.submit`

- 必选参数
  - `req_key` (`string`)：固定值 `jimeng_t2i_v40`
  - `prompt` (`string`)：生成提示词
- 可选参数
  - `image_urls` (`[]string`)：0~10 张参考图 URL
  - `size` (`int`)：默认 `4194304`，范围 `[1024*1024, 4096*4096]`
  - `width` (`int`)：需与 `height` 一起生效
  - `height` (`int`)：需与 `width` 一起生效
  - `scale` (`float`)：文本影响程度，默认 `0.5`，范围 `[0,1]`
  - `force_single` (`bool`)：是否强制单图，默认 `false`
  - `min_ratio` (`float`)：最小宽高比，默认 `1/3`
  - `max_ratio` (`float`)：最大宽高比，默认 `3`
- 返回字段
  - `data.task_id` (`string`)：异步任务 ID
  - `code` / `status` / `message` / `request_id` / `time_elapsed`

### 2) `jimeng.image.v40.get`

- 必选参数
  - `req_key` (`string`)：固定值 `jimeng_t2i_v40`
  - `task_id` (`string`)：提交接口返回的任务 ID
- 可选参数
  - `req_json` (`string`)：JSON 序列化字符串（可用于扩展字段透传）
- 返回字段
  - `data.status` (`string`)：任务状态
  - `data.image_urls` (`[]string|null`)：结果图片 URL
  - `data.binary_data_base64` (`[]string|null`)：base64 结果
  | `code` / `status` / `message` / `request_id` / `time_elapsed` | | | |

## 视频生成预设矩阵 (Video 3.0)

| 预设 (Preset) | 对应 ReqKey | 变体 (Variant) | 说明 |
| :--- | :--- | :--- | :--- |
| `t2v-720` | `jimeng_t2v_v30_720p` | `t2v` | 文生视频 720p |
| `t2v-1080` | `jimeng_t2v_v30_1080p` | `t2v` | 文生视频 1080p |
| `i2v-first` | `jimeng_i2v_first_v30_1080` | `i2v-first-frame` | 图生视频 (首帧) |
| `i2v-first-tail` | `jimeng_i2v_first_tail_v30_1080` | `i2v-first-tail` | 图生视频 (首尾帧) |
| `i2v-recamera` | `jimeng_i2v_recamera_v30` | `i2v-recamera` | 图生视频 (运镜) |

### 视频提交参数 (Video Submit)

- `prompt` (`string`)：提示词
- `frames` (`int`)：帧数
- `aspect_ratio` (`string`)：宽高比
- `image_urls` (`[]string`)：参考图 URL
- `template_id` (`string`)：模板 ID
- `camera_strength` (`string`)：运镜强度 (`weak`, `medium`, `strong`)
- `return_url` (`bool`)：固定为 `true`

## 状态机（统一枚举）

冻结状态枚举：

- `in_queue`
- `generating`
- `done`
- `not_found`
- `expired`
- `failed`

状态流转：

```text
in_queue -> generating -> done
in_queue -> not_found
generating -> expired
generating -> failed
```

说明：`failed` 作为客户端统一失败态纳入矩阵，便于与服务端异常码映射。
