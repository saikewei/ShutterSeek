package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"shutterseek/internal/config"
	"shutterseek/internal/db"
	"shutterseek/internal/handler"
	"shutterseek/internal/middleware"
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
	// TLS terminates in the nginx edge container. The app itself only serves
	// plain HTTP, so the Secure cookie flag must be selected explicitly.
	middleware.CookieSecure = os.Getenv("SHUTTERSEEK_COOKIE_SECURE") == "true"
	log.Printf("✓ Secure cookies: %t", middleware.CookieSecure)

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

	origSvc := service.NewOriginalService(cfg.Thumbnail.PhotosDir, cfg.Upload.UploadDir, "/tmp/shutterseek_previews")
	cache := &service.Cache{Redis: rdb}
	albumSvc := service.NewAlbumService(gormDB, cache)
	uploadSvc := service.NewUploadServiceWithOptions(gormDB, rdb, cfg.Upload.UploadDir, cfg.Thumbnail.OutputDir, service.UploadOptions{
		Workers:       cfg.Upload.Workers,
		BatchMaxFiles: cfg.Upload.BatchMaxFiles,
		MaxBatchBytes: cfg.Upload.MaxBatchBytes,
	})
	uploadSvc.Start()
	photoSvc := service.NewPhotoService(gormDB, cache, albumSvc)
	// The stats service pings the sidecar's health endpoint, which the caching
	// wrapper does not expose, so it gets the raw HTTP embedder.
	httpEmbedder := service.NewHTTPEmbedder(cfg.Embed.URL, cfg.Embed.Timeout(), cfg.Embed.Token)
	embedder := service.NewCachedEmbedder(httpEmbedder, rdb)
	searchSvc := service.NewSearchService(gormDB, embedder, cfg.Embed.MaxText)
	statsSvc := service.NewStatsService(gormDB, pool, rdb, httpEmbedder)
	h := &handler.Handler{
		Pool: pool, Redis: rdb, DB: gormDB,
		OrigSvc: origSvc, AlbumSvc: albumSvc, AuthSvc: authSvc,
		SearchSvc: searchSvc, UploadSvc: uploadSvc, PhotoSvc: photoSvc,
		StatsSvc: statsSvc, ThumbnailsDir: cfg.Thumbnail.OutputDir,
	}
	if os.Getenv("SHUTTERSEEK_CREATIONS_ENABLED") == "true" {
		address := os.Getenv("SHUTTERSEEK_AGENT_ADDRESS")
		if address == "" {
			address = "127.0.0.1:50051"
		}
		listen := os.Getenv("SHUTTERSEEK_CREATIONS_RPC_LISTEN")
		if listen == "" {
			listen = "127.0.0.1:8081"
		}
		creationSvc, err := service.StartCreationService(gormDB, cfg.Thumbnail.OutputDir, address, listen)
		if err != nil {
			log.Fatalf("creation service: %v", err)
		}
		h.CreationSvc = creationSvc
		defer creationSvc.Close()
	}
	r := router.Setup(h, cfg.Thumbnail.OutputDir, cfg.Model.Dir)

	// Only the nginx edge is allowed to supply client/proxy headers. Without
	// this, any client could spoof X-Forwarded-For.
	if raw := strings.TrimSpace(os.Getenv("SHUTTERSEEK_TRUSTED_PROXIES")); raw != "" {
		proxies := strings.Split(raw, ",")
		for i := range proxies {
			proxies[i] = strings.TrimSpace(proxies[i])
		}
		if err := r.SetTrustedProxies(proxies); err != nil {
			log.Printf("⚠ invalid SHUTTERSEEK_TRUSTED_PROXIES: %v (disabling proxy trust)", err)
			_ = r.SetTrustedProxies(nil)
		} else {
			log.Printf("✓ Trusted proxies: %s", raw)
		}
	} else {
		_ = r.SetTrustedProxies(nil)
	}

	// Server (TLS is terminated by nginx).
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Server.Port), Handler: r}

	go func() {
		log.Printf("🚀 Server listening on :%d (TLS terminated by nginx)", cfg.Server.Port)
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
	uploadSvc.Stop() // 清掉未落地的缓存失效请求并停掉后台 ticker
	log.Println("Server stopped")
}
