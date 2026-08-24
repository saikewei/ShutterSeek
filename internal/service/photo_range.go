package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"shutterseek/internal/model"
)

const RangeSelectLimit = 5000

var ErrAnchorNotFound = errors.New("anchor photos not found")

// RangeTooLargeError 携带实际计数，handler 用它返回 400 + count。
type RangeTooLargeError struct {
	Count int
}

func (e RangeTooLargeError) Error() string {
	return fmt.Sprintf("range too large: count=%d", e.Count)
}

type RangeParams struct {
	FromID  int64
	ToID    int64
	AlbumID string // "none" = uncategorized
	Month   string
	Role    string
}

// PhotoRange 返回两个锚点照片之间的全部照片 id（按列表序），SQL 与重构前逐字一致。
func (s *PhotoService) PhotoRange(ctx context.Context, p RangeParams) ([]int64, error) {
	var anchors []model.Photo
	if err := s.DB.WithContext(ctx).Select("id, taken_at").
		Where("id IN ?", []int64{p.FromID, p.ToID}).Find(&anchors).Error; err != nil {
		return nil, err
	}
	if len(anchors) != 2 {
		return nil, ErrAnchorNotFound
	}

	var a, b model.Photo
	if anchors[0].ID == p.FromID {
		a, b = anchors[0], anchors[1]
	} else {
		a, b = anchors[1], anchors[0]
	}
	at, bt := effectiveTakenAt(a.TakenAt), effectiveTakenAt(b.TakenAt)
	loT, loID, hiT, hiID := at, a.ID, bt, b.ID
	if photoTupleLess(bt, b.ID, at, a.ID) {
		loT, loID, hiT, hiID = bt, b.ID, at, a.ID
	}

	q := `SELECT id FROM photos
	      WHERE (COALESCE(taken_at, 'epoch'), id) >= (?, ?)
	        AND (COALESCE(taken_at, 'epoch'), id) <= (?, ?)`
	args := []interface{}{loT, loID, hiT, hiID}

	uncategorized := p.AlbumID == "none"
	if p.AlbumID != "" && !uncategorized {
		if albumID, err := strconv.ParseInt(p.AlbumID, 10, 64); err == nil && albumID > 0 {
			q += " AND id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)"
			args = append(args, albumID)
		}
	}
	if uncategorized {
		q += " AND id NOT IN (SELECT DISTINCT photo_id FROM album_photos)"
	}
	if p.Month != "" {
		if t, err := time.Parse("2006-01", p.Month); err == nil {
			q += " AND taken_at < ?"
			args = append(args, t.AddDate(0, 1, 0))
		}
	}
	if p.Role == "guest" {
		q += " AND id IN (SELECT ap.photo_id FROM album_photos ap JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true)"
	}
	q += " ORDER BY taken_at DESC NULLS LAST, id DESC LIMIT ?"
	args = append(args, RangeSelectLimit+1)

	var ids []int64
	if err := s.DB.WithContext(ctx).Raw(q, args...).Scan(&ids).Error; err != nil {
		return nil, err
	}
	if len(ids) > RangeSelectLimit {
		return nil, RangeTooLargeError{Count: len(ids)}
	}
	return ids, nil
}

func photoTupleLess(t1 time.Time, id1 int64, t2 time.Time, id2 int64) bool {
	if !t1.Equal(t2) {
		return t1.Before(t2)
	}
	return id1 < id2
}

func effectiveTakenAt(t time.Time) time.Time {
	if t.IsZero() {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return t
}
