package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadClientConfig_Success(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	content := `
[Client]
ServerURL = "https://example.com"
Token = "my-token"
ReportIP = "1.2.3.4"

[Connection]
ReconnectInterval = 10
MaxReconnectInterval = 120
HeartbeatInterval = 60

[Logging]
Level = "debug"
File = "/var/log/mb.log"

[Forwarder]
BufferSize = 65536
ConnectTimeout = 15
IdleTimeout = 600
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadClientConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadClientConfig error: %v", err)
	}

	if cfg.Client.ServerURL != "https://example.com" {
		t.Errorf("ServerURL = %q, want %q", cfg.Client.ServerURL, "https://example.com")
	}
	if cfg.Client.Token != "my-token" {
		t.Errorf("Token = %q, want %q", cfg.Client.Token, "my-token")
	}
	if cfg.Client.ReportIP != "1.2.3.4" {
		t.Errorf("ReportIP = %q, want %q", cfg.Client.ReportIP, "1.2.3.4")
	}
	if cfg.Connection.ReconnectInterval != 10 {
		t.Errorf("ReconnectInterval = %d, want 10", cfg.Connection.ReconnectInterval)
	}
	if cfg.Connection.MaxReconnectInterval != 120 {
		t.Errorf("MaxReconnectInterval = %d, want 120", cfg.Connection.MaxReconnectInterval)
	}
	if cfg.Connection.HeartbeatInterval != 60 {
		t.Errorf("HeartbeatInterval = %d, want 60", cfg.Connection.HeartbeatInterval)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Level = %q, want %q", cfg.Logging.Level, "debug")
	}
	if cfg.Forwarder.BufferSize != 65536 {
		t.Errorf("BufferSize = %d, want 65536", cfg.Forwarder.BufferSize)
	}
	if cfg.Forwarder.ConnectTimeout != 15 {
		t.Errorf("ConnectTimeout = %d, want 15", cfg.Forwarder.ConnectTimeout)
	}
	if cfg.Forwarder.IdleTimeout != 600 {
		t.Errorf("IdleTimeout = %d, want 600", cfg.Forwarder.IdleTimeout)
	}
}

func TestLoadClientConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	content := `
[Client]
Token = "x"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadClientConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadClientConfig error: %v", err)
	}

	if cfg.Client.ServerURL != "http://localhost:8080" {
		t.Errorf("ServerURL = %q, want default %q", cfg.Client.ServerURL, "http://localhost:8080")
	}
	if cfg.Connection.ReconnectInterval != 5 {
		t.Errorf("ReconnectInterval = %d, want default 5", cfg.Connection.ReconnectInterval)
	}
	if cfg.Connection.HeartbeatInterval != 30 {
		t.Errorf("HeartbeatInterval = %d, want default 30", cfg.Connection.HeartbeatInterval)
	}
	if cfg.Forwarder.BufferSize != 32768 {
		t.Errorf("BufferSize = %d, want default 32768", cfg.Forwarder.BufferSize)
	}
}

func TestLoadClientConfig_FileNotFound(t *testing.T) {
	_, err := LoadClientConfig("/nonexistent/path/config.toml")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestLoadClientConfig_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.toml")

	if err := os.WriteFile(cfgPath, []byte("{{{invalid"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := LoadClientConfig(cfgPath)
	if err == nil {
		t.Error("expected error for invalid TOML, got nil")
	}
}
