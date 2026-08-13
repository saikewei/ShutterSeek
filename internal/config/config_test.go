package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeMinimalConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEmbedConfigDefaults(t *testing.T) {
	t.Setenv("SHUTTERSEEK_EMBED_URL", "")
	t.Setenv("SHUTTERSEEK_EMBED_TIMEOUT_MS", "")
	t.Setenv("SHUTTERSEEK_EMBED_MAX_TEXT", "")
	t.Setenv("SHUTTERSEEK_EMBED_TOKEN", "")
	cfg, err := Load(writeMinimalConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Embed.URL != "http://127.0.0.1:8000" {
		t.Fatalf("URL = %q", cfg.Embed.URL)
	}
	if cfg.Embed.TimeoutMS != 10000 {
		t.Fatalf("TimeoutMS = %d", cfg.Embed.TimeoutMS)
	}
	if cfg.Embed.MaxText != 200 {
		t.Fatalf("MaxText = %d", cfg.Embed.MaxText)
	}
	if cfg.Embed.Timeout() != 10*time.Second {
		t.Fatalf("Timeout() = %v", cfg.Embed.Timeout())
	}
}

func TestEmbedConfigFromEnv(t *testing.T) {
	t.Setenv("SHUTTERSEEK_EMBED_URL", "http://embedding:8000")
	t.Setenv("SHUTTERSEEK_EMBED_TIMEOUT_MS", "5000")
	t.Setenv("SHUTTERSEEK_EMBED_MAX_TEXT", "100")
	t.Setenv("SHUTTERSEEK_EMBED_TOKEN", "tok123")
	cfg, err := Load(writeMinimalConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Embed.URL != "http://embedding:8000" || cfg.Embed.TimeoutMS != 5000 ||
		cfg.Embed.MaxText != 100 || cfg.Embed.Token != "tok123" {
		t.Fatalf("unexpected embed config: %+v", cfg.Embed)
	}
}

func TestUploadAndModelDirsFromEnv(t *testing.T) {
	t.Setenv("SHUTTERSEEK_UPLOAD_DIR", "/tmp/ss_uploads")
	t.Setenv("SHUTTERSEEK_MODELS_DIR", "/tmp/ss_models")
	cfg, err := Load(writeMinimalConfig(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Upload.UploadDir != "/tmp/ss_uploads" {
		t.Fatalf("upload dir = %q", cfg.Upload.UploadDir)
	}
	if cfg.Model.Dir != "/tmp/ss_models" {
		t.Fatalf("model dir = %q", cfg.Model.Dir)
	}
}
