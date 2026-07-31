package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"shutterseek/internal/config"
	"shutterseek/internal/db"
	"shutterseek/internal/handler"
	myredis "shutterseek/internal/redis"
	"shutterseek/internal/router"
	"shutterseek/internal/service"
)

func main() {
	// ── 配置 ──────────────────────────────────────────────
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx := context.Background()

	// ── PostgreSQL ────────────────────────────────────────
	pool, err := db.NewPGPool(ctx, cfg.Database.DSN())
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	log.Println("✓ PostgreSQL connected")

	// ── GORM ──────────────────────────────────────────────
	gormDB, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("gorm: %v", err)
	}
	log.Println("✓ GORM ready")

	// ── Redis (可选，失败不阻塞) ──────────────────────────
	rdb := myredis.NewClient(cfg.Redis)
	if err := myredis.Ping(ctx, rdb); err != nil {
		log.Printf("⚠ Redis unavailable: %v (continuing without cache)", err)
	} else {
		defer rdb.Close()
		log.Println("✓ Redis connected")
	}

	// ── Gin 引擎 ──────────────────────────────────────────
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// ── Auth ──────────────────────────────────────────────
	jwtSecret := os.Getenv("SHUTTERSEEK_JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("SHUTTERSEEK_JWT_SECRET is required")
	}
	authSvc := service.NewAuthService(gormDB, jwtSecret)

	// Seed initial admin if none exists
	adminPw := os.Getenv("SHUTTERSEEK_ADMIN_PASSWORD")
	if adminPw == "" {
		log.Fatal("SHUTTERSEEK_ADMIN_PASSWORD is required for admin seed")
	}
	if err := authSvc.SeedAdmin(adminPw); err != nil {
		log.Fatalf("seed admin: %v", err)
	}
	log.Println("✓ Admin user seeded")

	origSvc := service.NewOriginalService(cfg.Thumbnail.PhotosDir, "/tmp/shutterseek_previews")
	albumSvc := service.NewAlbumService(gormDB)
	h := &handler.Handler{Pool: pool, Redis: rdb, DB: gormDB, OrigSvc: origSvc, AlbumSvc: albumSvc, AuthSvc: authSvc}
	r := router.Setup(h, cfg.Thumbnail.OutputDir)

	// ── HTTP Server ───────────────────────────────────────
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	go func() {
		log.Printf("🚀 Server listening on :%d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	// ── 优雅退出 ──────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("Server stopped")
}
