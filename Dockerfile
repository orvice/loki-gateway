FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/loki-gateway ./cmd/loki-gateway

FROM alpine:3.21

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/loki-gateway /app/loki-gateway

ENV BUTTERFLY_CONFIG_TYPE=file \
    BUTTERFLY_CONFIG_FILE_PATH=/app/config/service.yaml \
    BUTTERFLY_TRACING_PROVIDER=http \
    BUTTERFLY_TRACING_ENDPOINT=127.0.0.1:4318


ENTRYPOINT ["/app/loki-gateway"]
