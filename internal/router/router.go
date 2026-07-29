package router

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/handler"
)

// Setup registers all routes and returns the Gin engine.
func Setup(h *handler.Handler, thumbDir string) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// API v1
	v1 := r.Group("/api/v1")
	{
		v1.GET("/photos", h.ListPhotos)
		v1.GET("/photos/:id/original", h.GetOriginal)
	}
	r.GET("/api/health", h.Health)

	// Thumbnails — served under /api/thumbnails/ to avoid Vite proxy issues
	if _, err := os.Stat(thumbDir); err == nil {
		r.Static("/api/thumbnails", thumbDir)
		r.Static("/thumbnails", thumbDir) // keep old path for direct backend access
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
