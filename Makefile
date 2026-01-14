.PHONY: all build server client linux darwin clean install uninstall test fmt tidy

VERSION := 2.0.0
BUILD_TIME := $(shell date +%Y%m%d%H%M%S)
LDFLAGS := -ldflags "-s -w"

all: build

build: server client

server:
	go build $(LDFLAGS) -o bin/server ./cmd/server

client:
	go build $(LDFLAGS) -o bin/client ./cmd/client

linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/messageboy-server-linux-amd64 ./cmd/server
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/messageboy-client-linux-amd64 ./cmd/client
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/messageboy-server-linux-arm64 ./cmd/server
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/messageboy-client-linux-arm64 ./cmd/client

darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/messageboy-server-darwin-amd64 ./cmd/server
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/messageboy-client-darwin-amd64 ./cmd/client
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/messageboy-server-darwin-arm64 ./cmd/server
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/messageboy-client-darwin-arm64 ./cmd/client

clean:
	rm -rf bin/*

test:
	go test -v ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

run-server:
	go run ./cmd/server --config configs/server.toml

run-client:
	go run ./cmd/client --config configs/client.toml

# 旧版本兼容（单文件转发器）
legacy:
	go build -o messageboy main.go

legacy-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o messageboy-linux main.go

install:
	mkdir -p /opt/messageboy
	cp bin/messageboy-server-linux-amd64 /opt/messageboy/messageboy-server
	cp configs/server.toml /opt/messageboy/
	systemctl daemon-reload

uninstall:
	systemctl stop messageboy-server || true
	systemctl disable messageboy-server || true
	rm -f /etc/systemd/system/messageboy-server.service
	rm -rf /opt/messageboy
	systemctl daemon-reload
