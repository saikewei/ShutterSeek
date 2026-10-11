package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"gorm.io/gorm"
	"shutterseek/internal/model"
)

var ErrCreationPhotoForbidden = errors.New("creation photo access denied")

// CreationActor is supplied by authenticated HTTP middleware or an owner lookup for RPC.
type CreationActor struct {
	ID   int64
	Role string
}

func (a CreationActor) valid() bool { return a.ID > 0 && (a.Role == "admin" || a.Role == "guest") }

func mergeCreationPhotos(parts ...[]int64) []int64 {
	seen := map[int64]bool{}
	for _, part := range parts {
		for _, id := range part {
			seen[id] = true
		}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
func creationPhotoHistory(c model.Creation) []int64 {
	var ids, history []int64
	_ = json.Unmarshal([]byte(c.PhotoIDs), &ids)
	_ = json.Unmarshal([]byte(jsonText(mapText(c.State)["photo_history"])), &history)
	return mergeCreationPhotos(ids, history)
}

// validateCreationPhotos limits guests in SQL; matching an ID is not authorization.
func validateCreationPhotos(ctx context.Context, db *gorm.DB, actor CreationActor, ids []int64) error {
	if !actor.valid() {
		return ErrCreationPhotoForbidden
	}
	if len(ids) == 0 {
		return ErrCreationInvalid
	}
	q := db.WithContext(ctx).Model(&model.Photo{}).Where("id IN ?", ids)
	if actor.Role == "guest" {
		q = q.Where("id IN (SELECT ap.photo_id FROM album_photos ap JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true)")
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		if actor.Role == "guest" {
			return ErrCreationPhotoForbidden
		}
		return ErrCreationInvalid
	}
	return nil
}

// History protects old messages, checkpoints and screenshots after album access is revoked.
func authorizeCreationWork(ctx context.Context, db *gorm.DB, actor CreationActor, c model.Creation) error {
	if !actor.valid() {
		return ErrCreationPhotoForbidden
	}
	if actor.Role == "admin" {
		return nil
	}
	ids := creationPhotoHistory(c)
	if mapText(c.State)["photo_history"] == nil {
		var versions []model.CreationVersion
		if err := db.WithContext(ctx).Select("used_photo_ids").Where("creation_id = ?", c.ID).Find(&versions).Error; err != nil {
			return err
		}
		for _, v := range versions {
			var used []int64
			_ = json.Unmarshal([]byte(v.UsedPhotoIDs), &used)
			ids = mergeCreationPhotos(ids, used)
		}
	}
	return validateCreationPhotos(ctx, db, actor, ids)
}
