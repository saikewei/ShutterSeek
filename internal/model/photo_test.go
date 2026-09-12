//go:build integration

package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"shutterseek/internal/config"
)

// closeTestDB 在测试结束时关掉这个测试自己的连接池。
// 每个测试都会 gorm.Open 一个新的池，而 database/sql 默认永久保留最多 2 条
// 空闲连接；几十个集成测试就能把 Postgres 的 max_connections（100）吃光，
// 报 SQLSTATE 53300（remaining connection slots are reserved）。
func closeTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	t.Cleanup(func() { sqlDB.Close() })
}

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()

	cfg, err := config.Load("../../config.yaml")
	require.NoError(t, err)

	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
	require.NoError(t, err)
	closeTestDB(t, db)
	return db
}

func TestPhotoCount(t *testing.T) {
	db := setupDB(t)

	var count int64
	err := db.Model(&Photo{}).Count(&count).Error
	require.NoError(t, err)
	t.Logf("Total photos: %d", count)
	assert.Greater(t, count, int64(0), "should have photos in database")
}

func TestPhotoByID(t *testing.T) {
	db := setupDB(t)

	var photo Photo
	err := db.Where("id = ?", 1).First(&photo).Error
	require.NoError(t, err)

	assert.Equal(t, int64(1), photo.ID)
	assert.NotEmpty(t, photo.FilePath)
	t.Logf("Photo 1: %s | %s %s", photo.FilePath, photo.CameraMake, photo.CameraModel)
}

func TestPhotoEmbeddingCount(t *testing.T) {
	db := setupDB(t)

	var count int64
	err := db.Model(&PhotoEmbedding{}).Count(&count).Error
	require.NoError(t, err)
	t.Logf("Total embeddings: %d", count)
	assert.Greater(t, count, int64(0), "should have embeddings in database")
}

func TestPhotoEmbeddingByID(t *testing.T) {
	db := setupDB(t)

	var emb PhotoEmbedding
	err := db.Where("photo_id = ?", 1).First(&emb).Error
	require.NoError(t, err)

	assert.Equal(t, int64(1), emb.PhotoID)
	t.Logf("Embedding for photo 1: vector present")
}

func TestPhotoPagination(t *testing.T) {
	db := setupDB(t)

	var photos []Photo
	err := db.Limit(10).Offset(0).Order("id").Find(&photos).Error
	require.NoError(t, err)

	assert.Len(t, photos, 10, "should return 10 photos")
	for _, p := range photos {
		assert.NotEmpty(t, p.FilePath)
	}
	t.Logf("First 10 photos: %d .. %d", photos[0].ID, photos[9].ID)
}

func TestPhotoWithTakenAt(t *testing.T) {
	db := setupDB(t)

	var count int64
	err := db.Model(&Photo{}).Where("taken_at IS NOT NULL").Count(&count).Error
	require.NoError(t, err)
	t.Logf("Photos with date: %d / total", count)
}

func BenchmarkPhotoCount(b *testing.B) {
	cfg, _ := config.Load("../../config.yaml")
	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var count int64
		db.Model(&Photo{}).Count(&count)
	}
}

func BenchmarkPhotoByID(b *testing.B) {
	cfg, _ := config.Load("../../config.yaml")
	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var photo Photo
		db.Where("id = ?", 1).First(&photo)
	}
}

// Test helpers compile and config loads correctly
func TestConfigLoad(t *testing.T) {
	cfg, err := config.Load("../../config.yaml")
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.Database.DSN())
	// Ensure sensitive values come from env, not config file
	assert.NotEmpty(t, cfg.Database.User, "DB user should come from .env.local")
	_ = context.Background()
}
