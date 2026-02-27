# Jimeng Relay 开发规范

本文档定义了 Jimeng Relay 项目的开发规范，旨在确保代码库的一致性、可维护性和可靠性。

## 1. 概述

Jimeng Relay 是一个高性能的 API 网关，对代码质量有严格要求。所有贡献者必须遵循本规范。

## 2. 代码风格

### 2.1 格式化
- 必须使用 `gofmt` 和 `goimports` 进行代码格式化。
- 建议在编辑器中开启保存时自动格式化。

### 2.2 导入组织
- 导入包应分为三组，用空行分隔：
  1. 标准库包
  2. 第三方包
  3. 项目内部包
- 示例：
  ```go
  import (
      "context"
      "fmt"

      "github.com/google/uuid"
      "log/slog"

      "github.com/jimeng-relay/server/internal/errors"
  )
  ```

### 2.3 行长度限制
- 尽量保持单行长度在 120 字符以内。
- 超过限制时应进行合理换行。

## 3. 错误处理

### 3.1 避免 Panic
- **严禁**在业务逻辑中使用 `panic`。
- 必须通过返回 `error` 来处理异常情况。
- 在 `main` 函数或顶层命令中，可以使用 `log.Fatal` 或 `cobra.CheckErr`。

### 3.2 错误包装
- 使用项目自定义的 `internal/errors` 包创建错误。
- 必须包含明确的错误码（Code）和上下文信息。
- **正确示例**：
  ```go
  if err != nil {
      return internalerrors.New(internalerrors.ErrUpstreamFailed, "submit request to upstream", err)
  }
  ```
- **错误示例**：
  ```go
  if err != nil {
      return err // 丢失上下文
  }
  // 或者
  if err != nil {
      return errors.New("failed") // 丢失错误码
  }
  ```

### 3.3 错误码定义
- 错误码应在 `server/internal/errors/errors.go` 中统一定义。
- 常用错误码：`ErrAuthFailed`, `ErrValidationFailed`, `ErrRateLimited`, `ErrUpstreamFailed`, `ErrInternalError`。
### 3.4 错误处理示例 (Good vs Bad)

**Bad (丢失上下文和错误码):**
```go
func (s *Service) DoSomething() error {
    err := s.repo.Save(ctx, data)
    if err != nil {
        return err // ❌ 丢失了是在哪个步骤失败的，且没有错误码
    }
    return nil
}
```

**Good (包装错误并添加上下文):**
```go
func (s *Service) DoSomething() error {
    err := s.repo.Save(ctx, data)
    if err != nil {
        return internalerrors.New(internalerrors.ErrDatabaseError, "save data to repository", err) // ✅ 包含错误码和上下文
    }
    return nil
}
```


## 4. 日志记录

### 4.1 结构化日志
- 必须使用 `log/slog` 进行结构化日志记录。
- 严禁使用 `fmt.Printf` 或 `log.Printf` 记录业务日志。

### 4.2 日志级别
- `Debug`: 开发调试信息。
- `Info`: 关键业务流程信息（如请求完成）。
- `Warn`: 非致命异常，可恢复的错误。
- `Error`: 严重错误，需要人工介入。

### 4.3 敏感信息脱敏
- 日志中严禁出现明文密钥（Secret Key）、签名（Signature）等敏感信息。
- 使用 `logging.RedactingHandler` 自动处理脱敏。
- **正确示例**：
  ```go
  logger.InfoContext(ctx, "request completed", 
      "latency_ms", latencyMs, 
      "status", statusCode,
  )
  ```
### 4.4 日志记录示例 (Good vs Bad)

**Bad (非结构化日志):**
```go
log.Printf("request failed for user %s: %v", userID, err) // ❌ 难以解析和搜索
```

**Good (结构化日志):**
```go
logger.ErrorContext(ctx, "request failed", 
    "user_id", userID, 
    "error", err,
) // ✅ 结构化，包含上下文，易于搜索
```


## 5. 命名约定

### 5.1 包名
- 简短、有意义、全小写，不使用下划线或驼峰。
- 示例：`handler`, `repository`, `apikey`。

### 5.2 变量与函数名
- 使用驼峰命名法（CamelCase）。
- 导出变量/函数首字母大写，私有变量/函数首字母小写。
- 缩写词保持一致大小写（如 `APIKey` 而非 `ApiKey`）。

### 5.3 接口命名
- 接口名通常以 `er` 结尾。
- 示例：`Reader`, `Writer`, `IdempotencyRecordRepository`。
### 5.4 常量名
- 使用驼峰命名法（CamelCase）。
- 示例：`ErrAuthFailed`, `DefaultTimeout`。


## 6. 注释规范

### 6.1 函数与包注释
- 所有导出的函数、结构体、接口必须有注释。
- 注释应以被描述对象的名称开头。
- 示例：
  ```go
  // NewService 创建一个新的 API Key 服务实例。
  func NewService(repo repository.APIKeyRepository) *Service { ... }
  ```

### 6.2 TODO 格式
- 使用 `// TODO(username): description` 格式。

## 7. 测试规范

### 7.1 文件与函数命名
- 测试文件以 `_test.go` 结尾。
- 测试函数以 `Test` 开头，如 `TestSubmitHandler_Success`。

### 7.2 表格驱动测试
- 对于逻辑复杂的函数，优先使用表格驱动测试（Table-Driven Tests）。
- 示例：
  ```go
  func TestSomething(t *testing.T) {
      tests := []struct {
          name string
          input string
          want  string
      }{
          {"case1", "in1", "out1"},
          {"case2", "in2", "out2"},
      }
      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              // ...
          })
      }
  }
  ```

### 7.3 Mock 使用
- 优先使用接口进行依赖注入，并在测试中实现简单的 Mock 结构体。
- 避免使用过于复杂的 Mock 框架。

### 7.4 竞态检测
- 提交代码前必须运行 `go test -race ./...`。

## 8. 最佳实践总结

| 场景 | 推荐做法 | 避免做法 |
| :--- | :--- | :--- |
| 错误处理 | 返回 `internalerrors.Error` | `panic` 或返回裸 `error` |
| 日志 | `logger.InfoContext(ctx, ...)` | `fmt.Println` |
| 并发 | 使用 `sync.Mutex` 或 `chan` | 共享变量无保护 |
| 配置 | 环境变量 + `.env` | 硬编码常量 |
| 依赖 | 构造函数注入接口 | 全局变量单例 |
