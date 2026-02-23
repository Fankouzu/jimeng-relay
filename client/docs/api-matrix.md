# 即梦AI图片生成4.0 API矩阵（冻结版）

## 范围与冻结约束

- 仅覆盖即梦AI图片生成4.0（`req_key=jimeng_t2i_v40`）。
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
| `jimeng.image.v40.get` | `CVSync2AsyncGetResult` | `jimeng_t2i_v40` | 查询任务状态与结果 |

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
  - `code` / `status` / `message` / `request_id` / `time_elapsed`

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
