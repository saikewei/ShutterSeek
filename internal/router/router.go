package router

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"shutterseek/internal/handler"
)

// Setup registers all routes and returns the Gin engine.
func Setup(pool *pgxpool.Pool, rdb *redis.Client, thumbDir string) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	h := &handler.Handler{Pool: pool, Redis: rdb}

	// API
	r.GET("/api/health", h.Health)

	// Thumbnails static
	if _, err := os.Stat(thumbDir); err == nil {
		r.Static("/thumbnails", thumbDir)
		log.Printf("✓ Thumbnails: %s", thumbDir)
	} else {
		log.Printf("⚠ Thumbnails dir not found: %s", thumbDir)
	}

	// SPA frontend
	frontendDir := "frontend/dist"
	if _, err := os.Stat(frontendDir); err == nil {
		r.StaticFile("/", frontendDir+"/index.html")
		r.Static("/assets", frontendDir+"/assets")
		r.Static("/favicon.ico", frontendDir+"/favicon.ico")
		r.NoRoute(func(c *gin.Context) {
			c.File(frontendDir + "/index.html")
		})
		log.Printf("✓ Frontend: %s", frontendDir)
	}

	return r
}
