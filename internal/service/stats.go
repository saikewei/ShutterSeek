package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"shutterseek/internal/model"
)

// pingerTimeout bounds every health probe so a hanging dependency cannot hold
// the admin console open.
const pingerTimeout = 2 * time.Second

// The embeddings count is a full scan of a table holding 1024-dim vectors
// (~450ms measured at 68k rows), so the counters are cached. Health and uptime
// are deliberately NOT cached: the console's refresh button has to tell the
// truth about a dependency that just went away.
const (
	KeyAdminStats = "cache:admin_stats:v1"
	TTLAdminStats = 60 * time.Second
)

// StatsCounts is the cached half of the snapshot.
type StatsCounts struct {
	Photos         int64 `json:"photos"`
	Embeddings     int64 `json:"embeddings"`
	Albums         int64 `json:"albums"`
	PublicAlbums   int64 `json:"public_albums"`
	Users          int64 `json:"users"`
	Admins         int64 `json:"admins"`
	InvitesPending int64 `json:"invites_pending"`
	InvitesTotal   int64 `json:"invites_total"`
	Logs           int64 `json:"logs"`
}

// Stats is the read-only snapshot rendered by the admin console.
type Stats struct {
	StatsCounts
	DBOK          bool      `json:"db_ok"`
	RedisOK       bool      `json:"redis_ok"`
	EmbedOK       bool      `json:"embed_ok"`
	StartedAt     time.Time `json:"started_at"`
	UptimeSeconds int64     `json:"uptime_seconds"`
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
	return Stats{
		StatsCounts:   s.counts(ctx),
		DBOK:          s.probePool(ctx),
		RedisOK:       s.probeRedis(ctx),
		EmbedOK:       s.probeEmbed(ctx),
		StartedAt:     s.started,
		UptimeSeconds: int64(time.Since(s.started).Seconds()),
	}
}

// counts returns the cached counters, refilling them when the cache is cold or
// absent. A nil database reports all zeros.
func (s *StatsService) counts(ctx context.Context) StatsCounts {
	if s.Redis != nil {
		if data, err := s.Redis.Get(ctx, KeyAdminStats).Bytes(); err == nil {
			var cached StatsCounts
			if json.Unmarshal(data, &cached) == nil {
				return cached
			}
		}
	}

	var out StatsCounts
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

	if s.Redis != nil {
		if data, err := json.Marshal(out); err == nil {
			s.Redis.Set(ctx, KeyAdminStats, data, TTLAdminStats)
		}
	}

	return out
}

func (s *StatsService) probePool(ctx context.Context) bool {
	if s.Pool == nil {
		return false
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingerTimeout)
	defer cancel()
	return s.Pool.Ping(pingCtx) == nil
}

func (s *StatsService) probeRedis(ctx context.Context) bool {
	if s.Redis == nil {
		return false
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingerTimeout)
	defer cancel()
	return s.Redis.Ping(pingCtx).Err() == nil
}

func (s *StatsService) probeEmbed(ctx context.Context) bool {
	pinger, ok := s.Embed.(embedderPinger)
	if !ok {
		return false
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingerTimeout)
	defer cancel()
	return pinger.Ping(pingCtx) == nil
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
