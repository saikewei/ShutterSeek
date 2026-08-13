package router

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/handler"
	"shutterseek/internal/middleware"
)

// Setup registers all routes and returns the Gin engine.
func Setup(h *handler.Handler, thumbDir, modelsDir string) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// Public routes (no auth required)
	v1Public := r.Group("/api/v1")
	{
		v1Public.POST("/auth/login", h.Login)
		v1Public.POST("/invites/redeem", h.RedeemInvite)
		v1Public.GET("/invites/validate/:code", h.ValidateInvite)
	}

	// Authenticated routes
	v1 := r.Group("/api/v1")
	v1.Use(middleware.AuthRequired(h.AuthSvc))
	{
		v1.GET("/photos", h.ListPhotos)
		v1.GET("/photos/dates", h.PhotoDates)
		v1.GET("/photos/:id/original", h.GetOriginal)
		v1.GET("/albums", h.ListAlbums)
		v1.GET("/albums/:id", h.GetAlbum)
		v1.GET("/albums/:id/dates", h.AlbumDates)
		v1.GET("/albums/:id/photos", h.ListAlbumPhotos)
		v1.GET("/search", h.Search)
		v1.GET("/auth/me", h.Me)
		v1.POST("/auth/logout", h.Logout)

		// Admin only
		admin := v1.Group("")
		admin.Use(middleware.AdminOnly())
		{
			admin.POST("/albums", h.CreateAlbum)
			admin.PUT("/albums/:id", h.UpdateAlbum)
			admin.DELETE("/albums/:id", h.DeleteAlbum)
			admin.POST("/albums/:id/photos", h.BatchAddPhotos)
			admin.DELETE("/albums/:id/photos", h.BatchRemovePhotos)
			admin.DELETE("/albums/:id/photos/:photo_id", h.RemoveAlbumPhoto)
			admin.POST("/invites", h.CreateInvite)
			admin.GET("/invites", h.ListInvites)
			admin.DELETE("/invites/:id", h.DeleteInvite)
			admin.GET("/auth/logs", h.ListLogs)
			admin.GET("/photos/range", h.PhotoRange)
			admin.POST("/photos/upload", h.Upload)
		}
	}
	r.GET("/api/health", h.Health)

	// 客户端推理模型（fp16 ONNX，1.2GB，长缓存）
	r.GET("/models/vision_encoder/model.onnx", func(c *gin.Context) {
		p := filepath.Join(modelsDir, "vision_encoder", "model.onnx")
		if _, err := os.Stat(p); err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.File(p)
	})

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
		// onnxruntime-web 的 wasm/mjs（构建时从 public/ort 拷贝进 dist/ort）
		ortDir := filepath.Join(frontendDir, "ort")
		if _, err := os.Stat(ortDir); err == nil {
			r.Static("/ort", ortDir)
			log.Printf("✓ ORT wasm: %s", ortDir)
		}
		r.NoRoute(func(c *gin.Context) {
			c.File(frontendDir + "/index.html")
		})
		log.Printf("✓ Frontend: %s", frontendDir)
	}

	return r
}
