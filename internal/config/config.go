package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Redis     RedisConfig     `yaml:"redis"`
	Thumbnail ThumbnailConfig `yaml:"thumbnail"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type ThumbnailConfig struct {
	OutputDir string `yaml:"output_dir"`
}

func Load(path string) (*Config, error) {
	// Load .env.local if present (development)
	loadDotEnv(".env.local")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{
		Server: ServerConfig{Port: 8080, Mode: "debug"},
		Database: DatabaseConfig{
			Host: "postgres-main", Port: 5432, SSLMode: "disable",
		},
		Redis: RedisConfig{
			Host: "redis", Port: 6379, DB: 2,
		},
		Thumbnail: ThumbnailConfig{
			OutputDir: "/thumbnails/.thumbnails_1080",
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyEnv(cfg)
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("SHUTTERSEEK_SERVER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = n
		}
	}
	if v := os.Getenv("SHUTTERSEEK_SERVER_MODE"); v != "" {
		cfg.Server.Mode = v
	}
	if v := os.Getenv("SHUTTERSEEK_DB_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("SHUTTERSEEK_DB_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Database.Port = n
		}
	}
	if v := os.Getenv("SHUTTERSEEK_DB_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := os.Getenv("SHUTTERSEEK_DB_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("SHUTTERSEEK_DB_NAME"); v != "" {
		cfg.Database.DBName = v
	}
	if v := os.Getenv("SHUTTERSEEK_DB_SSLMODE"); v != "" {
		cfg.Database.SSLMode = v
	}
	if v := os.Getenv("SHUTTERSEEK_REDIS_HOST"); v != "" {
		cfg.Redis.Host = v
	}
	if v := os.Getenv("SHUTTERSEEK_REDIS_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Redis.Port = n
		}
	}
	if v := os.Getenv("SHUTTERSEEK_REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
	if v := os.Getenv("SHUTTERSEEK_REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Redis.DB = n
		}
	}
	if v := os.Getenv("SHUTTERSEEK_THUMBNAILS_DIR"); v != "" {
		cfg.Thumbnail.OutputDir = v
	}
}

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // file doesn't exist, skip
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode)
}
