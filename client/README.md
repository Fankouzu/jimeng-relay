# Jimeng Relay CLI Client

Jimeng Relay CLI client is a command-line tool for interacting with the Jimeng Relay service. It allows you to submit image generation tasks, query their status, wait for completion, and download the results.

## Installation

To install the Jimeng Relay CLI client, ensure you have Go installed and run:

```bash
go install github.com/jimeng-relay/client@latest
```

This will install the `jimeng` binary to your `$GOPATH/bin` directory.

## Configuration

The client can be configured using environment variables.

| Environment Variable | Description | Default Value |
|----------------------|-------------|---------------|
| `VOLC_ACCESSKEY`     | Volcengine Access Key (Required) | - |
| `VOLC_SECRETKEY`     | Volcengine Secret Key (Required) | - |
| `VOLC_REGION`        | Volcengine Region | `cn-north-1` |
| `VOLC_HOST`          | Volcengine API Host | `visual.volcengineapi.com` |
| `VOLC_TIMEOUT`       | API Request Timeout | `30s` |

## CLI Commands

The binary name is `jimeng`.

### submit

Submit a new image generation task.

**Example:**
```bash
./jimeng submit --prompt "a beautiful sunset over the mountains"
```

**Dry-run Example:**
```bash
./jimeng submit --prompt "test" --dry-run
```

### query

Query the status of a submitted task.

**Example:**
```bash
./jimeng query --task-id "your-task-id"
```

### wait

Wait for a task to complete (polling).

**Example:**
```bash
./jimeng wait --task-id "your-task-id"
```

### download

Download the result of a completed task.

**Example:**
```bash
./jimeng download --task-id "your-task-id" --output "result.png"
```

## Error Handling Guide

The client returns non-zero exit codes on failure. Common errors include:

- **Authentication Errors:** Check your `VOLC_ACCESSKEY` and `VOLC_SECRETKEY`.
- **Network Errors:** Check your internet connection and `VOLC_HOST` configuration.
- **Validation Errors:** Ensure your prompt and other parameters meet the API requirements.
- **Timeout Errors:** Increase `VOLC_TIMEOUT` if the network is slow.

## API Reference

For detailed information about the underlying API, please refer to the official documentation:
[Volcengine Jimeng API Documentation](https://www.volcengine.com/docs/85621/1817045)
