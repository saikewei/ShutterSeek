package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Database  DatabaseConfig  `yaml:"database"`
	Scanner   ScannerConfig   `yaml:"scanner"`
	Thumbnail ThumbnailConfig `yaml:"thumbnail"`
}

type DatabaseConfig struct {
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	User           string `yaml:"user"`
	Password       string `yaml:"password"`
	DBName         string `yaml:"dbname"`
	SSLMode        string `yaml:"sslmode"`
	MaxConnections int    `yaml:"max_connections"`
}

type ScannerConfig struct {
	PhotosDir  string   `yaml:"photos_dir"`
	Workers    int      `yaml:"workers"`
	BatchSize  int      `yaml:"batch_size"`
	Extensions []string `yaml:"extensions"`
	SkipVideo  bool     `yaml:"skip_video"`
}

type ThumbnailConfig struct {
	OutputDir string `yaml:"output_dir"`
	Size      int    `yaml:"size"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{
		Database: DatabaseConfig{
			Host:           "postgres-main",
			Port:           5432,
			SSLMode:        "disable",
			MaxConnections: 20,
		},
		Scanner: ScannerConfig{
			PhotosDir: "/photos",
			Workers:   8,
			BatchSize: 50,
			SkipVideo: true,
			Extensions: []string{
				".arw", ".nef", ".rw2", ".dng", ".cr2", ".cr3",
				".orf", ".raf", ".pef", ".sr2", ".srf",
				".jpg", ".jpeg", ".png", ".tif", ".tiff",
				".heic", ".heif", ".hif",
			},
		},
		Thumbnail: ThumbnailConfig{
			OutputDir: "/workspaces/ShutterSeek/thumbnails",
			Size:      1080,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Env overrides
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Database.Port)
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.Database.DBName = v
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		// Full DSN override
		cfg.Database.Host = ""
	}

	// Normalize extensions to lowercase
	for i, ext := range cfg.Scanner.Extensions {
		cfg.Scanner.Extensions[i] = strings.ToLower(ext)
	}

	return cfg, nil
}

func (d *DatabaseConfig) DSN() string {
	if os.Getenv("DATABASE_URL") != "" {
		return os.Getenv("DATABASE_URL")
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode,
	)
}
