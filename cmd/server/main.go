package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/gin-gonic/gin"
	libdnsalidns "github.com/libdns/alidns"
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
	embedder := service.NewCachedEmbedder(
		service.NewHTTPEmbedder(cfg.Embed.URL, cfg.Embed.Timeout(), cfg.Embed.Token),
		rdb,
	)
	searchSvc := service.NewSearchService(gormDB, embedder, cfg.Embed.MaxText)
	h := &handler.Handler{
		Pool: pool, Redis: rdb, DB: gormDB,
		OrigSvc: origSvc, AlbumSvc: albumSvc, AuthSvc: authSvc,
		SearchSvc: searchSvc,
	}
	r := router.Setup(h, cfg.Thumbnail.OutputDir)

	// ── Server (HTTP or HTTPS via Let's Encrypt DNS-01) ──
	tlsEnabled := os.Getenv("SHUTTERSEEK_TLS_ENABLED") == "true"
	srv := &http.Server{Handler: r}

	if tlsEnabled {
		setupTLS(srv, r)
	} else {
		srv.Addr = fmt.Sprintf(":%d", cfg.Server.Port)
	}

	go func() {
		var err error
		if tlsEnabled {
			log.Printf("🚀 HTTPS server ready on %s", srv.Addr)
			err = srv.ListenAndServeTLS("", "")
		} else {
			log.Printf("🚀 Server listening on :%d", cfg.Server.Port)
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
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

// setupTLS configures certmagic with the Alibaba Cloud DNS-01 solver and
// binds srv to the HTTPS port. Certificates are issued and auto-renewed.
func setupTLS(srv *http.Server, handler http.Handler) {
	domain := os.Getenv("SHUTTERSEEK_TLS_DOMAIN")
	if domain == "" {
		log.Fatal("SHUTTERSEEK_TLS_DOMAIN is required when SHUTTERSEEK_TLS_ENABLED=true")
	}
	tlsPort := 8443
	if p := os.Getenv("SHUTTERSEEK_TLS_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			tlsPort = n
		}
	}
	certDir := os.Getenv("SHUTTERSEEK_CERT_DIR")
	if certDir == "" {
		log.Fatal("SHUTTERSEEK_CERT_DIR is required when TLS is enabled")
	}
	aliKey := os.Getenv("ALIYUN_ACCESS_KEY_ID")
	aliSecret := os.Getenv("ALIYUN_ACCESS_KEY_SECRET")
	if aliKey == "" || aliSecret == "" {
		log.Fatal("ALIYUN_ACCESS_KEY_ID and ALIYUN_ACCESS_KEY_SECRET are required for the DNS-01 challenge")
	}

	certmagic.Default.Storage = &certmagic.FileStorage{Path: certDir}
	certmagic.DefaultACME.DNS01Solver = &certmagic.DNS01Solver{
		DNSManager: certmagic.DNSManager{
			DNSProvider: &libdnsalidns.Provider{
				CredentialInfo: libdnsalidns.CredentialInfo{
					AccessKeyID:     aliKey,
					AccessKeySecret: aliSecret,
				},
			},
		},
	}

	cm := certmagic.NewDefault()
	if err := cm.ManageAsync(context.Background(), []string{domain}); err != nil {
		log.Fatalf("certmagic manage: %v", err)
	}

	srv.Addr = fmt.Sprintf(":%d", tlsPort)
	srv.Handler = handler
	srv.TLSConfig = cm.TLSConfig()
	middleware.CookieSecure = true
	log.Printf("✓ TLS enabled — domain %s, certs dir %s", domain, certDir)
}
