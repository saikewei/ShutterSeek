package service

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"shutterseek/internal/model"
)

// pingerTimeout bounds every health probe so a hanging dependency cannot hold
// the admin console open.
const pingerTimeout = 2 * time.Second

// Stats is the read-only snapshot rendered by the admin console.
type Stats struct {
	Photos         int64     `json:"photos"`
	Embeddings     int64     `json:"embeddings"`
	Albums         int64     `json:"albums"`
	PublicAlbums   int64     `json:"public_albums"`
	Users          int64     `json:"users"`
	Admins         int64     `json:"admins"`
	InvitesPending int64     `json:"invites_pending"`
	InvitesTotal   int64     `json:"invites_total"`
	Logs           int64     `json:"logs"`
	DBOK           bool      `json:"db_ok"`
	RedisOK        bool      `json:"redis_ok"`
	EmbedOK        bool      `json:"embed_ok"`
	StartedAt      time.Time `json:"started_at"`
	UptimeSeconds  int64     `json:"uptime_seconds"`
}

// embedderPinger is deliberately separate from Embedder: the search service's
// test doubles only implement Embed, and adding a method to that interface
// would break them for no reason.
type embedderPinger interface {
	Ping(ctx context.Context) error
}

// StatsService reports counters and dependency health for the admin console.
type StatsService struct {
	DB      *gorm.DB
	Pool    *pgxpool.Pool
	Redis   *goredis.Client
	Embed   Embedder
	started time.Time
}

func NewStatsService(db *gorm.DB, pool *pgxpool.Pool, rdb *goredis.Client, embed Embedder) *StatsService {
	return &StatsService{DB: db, Pool: pool, Redis: rdb, Embed: embed, started: time.Now()}
}

// Snapshot never fails: the console must still render when a dependency is
// down, so failures surface as false/zero fields rather than an error. Like
// Cache, every collaborator may be nil and is then simply reported as down.
func (s *StatsService) Snapshot(ctx context.Context) Stats {
	out := Stats{
		StartedAt:     s.started,
		UptimeSeconds: int64(time.Since(s.started).Seconds()),
	}

	if s.DB != nil {
		out.Photos = countRows(ctx, s.DB.Model(&model.Photo{}))
		out.Embeddings = countRows(ctx, s.DB.Model(&model.PhotoEmbedding{}))
		out.Albums = countRows(ctx, s.DB.Model(&model.Album{}))
		out.PublicAlbums = countRows(ctx, s.DB.Model(&model.Album{}).Where("is_public = true"))
		out.Users = countRows(ctx, s.DB.Model(&model.User{}))
		out.Admins = countRows(ctx, s.DB.Model(&model.User{}).Where("role = ?", "admin"))
		out.InvitesTotal = countRows(ctx, s.DB.Model(&model.InviteCode{}))
		out.InvitesPending = countRows(ctx, s.DB.Model(&model.InviteCode{}).
			Where("used_by IS NULL AND expires_at > now()"))
		out.Logs = countRows(ctx, s.DB.Model(&model.UserLog{}))
	}

	if s.Pool != nil {
		pingCtx, cancel := context.WithTimeout(ctx, pingerTimeout)
		out.DBOK = s.Pool.Ping(pingCtx) == nil
		cancel()
	}

	if s.Redis != nil {
		pingCtx, cancel := context.WithTimeout(ctx, pingerTimeout)
		out.RedisOK = s.Redis.Ping(pingCtx).Err() == nil
		cancel()
	}

	if pinger, ok := s.Embed.(embedderPinger); ok {
		pingCtx, cancel := context.WithTimeout(ctx, pingerTimeout)
		out.EmbedOK = pinger.Ping(pingCtx) == nil
		cancel()
	}

	return out
}

// countRows reports 0 on error: a count that cannot be read is not worth
// failing the whole snapshot over.
func countRows(ctx context.Context, q *gorm.DB) int64 {
	var n int64
	if err := q.WithContext(ctx).Count(&n).Error; err != nil {
		return 0
	}
	return n
}
