# Build backend
FROM golang:1.25-alpine AS builder
WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o messageboy-server ./cmd/server

# Final stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/messageboy-server .

# Expose port
EXPOSE 8080

# Run application
CMD ["./messageboy-server", "--config", "configs/server.toml"]
