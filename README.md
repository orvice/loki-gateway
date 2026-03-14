# loki-gateway

## Structure

```text
.
├── cmd/loki-gateway/main.go
├── config/service.yaml
├── .env.example
└── go.mod
```

## Quick Start

1. Export env vars from `.env.example`.
2. Start local dependencies (for example, Redis on `127.0.0.1:6379`).
3. Run:

```bash
make run
```

## Make Targets

```bash
make run    # run service
make test   # run unit tests
make build  # build binary to ./bin/loki-gateway
```

## Loki Endpoints

- `POST /loki/api/v1/push`
- `GET /loki/api/v1/query`
- `GET /loki/api/v1/query_range`
- `GET /loki/api/v1/labels`
- `GET /loki/api/v1/label/{name}/values`
