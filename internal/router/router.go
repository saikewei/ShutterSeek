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
		v1.GET("/albums", h.ListAlbums)
		v1.POST("/albums", h.CreateAlbum)
		v1.GET("/albums/:id", h.GetAlbum)
		v1.PUT("/albums/:id", h.UpdateAlbum)
		v1.DELETE("/albums/:id", h.DeleteAlbum)
		v1.GET("/albums/:id/photos", h.ListAlbumPhotos)
		v1.POST("/albums/:id/photos", h.BatchAddPhotos)
		v1.DELETE("/albums/:id/photos/:photo_id", h.RemoveAlbumPhoto)
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
