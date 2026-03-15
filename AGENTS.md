# AGENTS.md

This file provides guidance to agents when working with code in this repository.

## Build/Test Commands
```bash
make run              # Run service locally (requires Redis on 127.0.0.1:6379)
make test             # Run all tests
make build            # Build binary to ./bin/loki-gateway
go test -run TestName ./internal/forwarder/...  # Run single test
```

## Architecture
Loki gateway that routes push/query requests to multiple Loki backends based on label matching.

```
Request → HTTP Handler → Service (routing) → Forwarder → Loki targets
```

- [`internal/config/loki.go`](internal/config/loki.go) - Target definitions and routing rules
- [`internal/routing/matcher.go`](internal/routing/matcher.go) - Label-based routing logic
- [`internal/service/push.go`](internal/service/push.go) - Push request handling with async forwarding
- [`internal/forwarder/client.go`](internal/forwarder/client.go) - HTTP client for downstream requests

## Critical Patterns

- **Async push forwarding**: Uses `context.WithoutCancel(ctx)` in [`internal/service/push.go:62`](internal/service/push.go:62) to allow goroutines to complete even if request context is cancelled
- **Response body lifecycle**: `cancelOnCloseReadCloser` wrapper in [`internal/forwarder/client.go:25-34`](internal/forwarder/client.go:25) ensures context is cancelled when response body is closed
- **Header override**: Target `ExtraHeaders` override (not merge) incoming request headers
- **Framework**: Uses `butterfly.orx.me/core` framework with YAML config, Gin router, and Redis store

## Config Requirements
- `loki.default_target` must reference an existing target name
- `loki.targets` requires at least one target with `name` and `url`
- `timeout_ms` defaults to 3000ms if not specified
