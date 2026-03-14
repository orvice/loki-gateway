SERVICE := loki-gateway
CMD_DIR := ./cmd/$(SERVICE)
BIN_DIR := ./bin
BIN := $(BIN_DIR)/$(SERVICE)

.PHONY: run test build

run:
	go run $(CMD_DIR)

test:
	go test ./...

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN) $(CMD_DIR)
