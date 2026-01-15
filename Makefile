.PHONY: docker client

# Docker image build for server
docker:
	@echo "Building Docker image..."
	@docker build -t messageboy-server:latest .

# Build client for linux amd64
client:
	@echo "Building messageboy-client-linux-amd64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o bin/messageboy-client-linux-amd64 ./cmd/client
