package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"shutterseek/internal/model"
)

var ErrCreationSharingUnavailable = errors.New("creation sharing schema unavailable")

type CreationShareRequest struct {
	VersionID   string `json:"version_id"`
	BaseVersion string `json:"base_version"`
}
type SharedCreation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	VersionID string    `json:"version_id"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *CreationService) SharingEnabled() bool { return s.sharingEnabled }

func (s *CreationService) OwnerShare(ctx context.Context, actor CreationActor, id string) (*model.CreationShare, error) {
	if !actor.valid() {
		return nil, ErrCreationPhotoForbidden
	}
	if !s.sharingEnabled {
		return nil, ErrCreationSharingUnavailable
	}
	var c model.Creation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, actor.ID).First(&c).Error; err != nil {
		return nil, err
	}
	var share model.CreationShare
	err := s.db.WithContext(ctx).Where("creation_id = ?", id).First(&share).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &share, err
}
func (s *CreationService) EnableShare(ctx context.Context, actor CreationActor, id string, req CreationShareRequest) (model.CreationShare, error) {
	var share model.CreationShare
	if !actor.valid() {
		return share, ErrCreationPhotoForbidden
	}
	if !s.sharingEnabled {
		return share, ErrCreationSharingUnavailable
	}
	if req.VersionID == "" {
		return share, ErrCreationInvalid
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c model.Creation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, actor.ID).First(&c).Error; err != nil {
			return err
		}
		if c.CurrentVersion != req.BaseVersion {
			return ErrCreationConflict
		}
		if err := authorizeCreationWork(ctx, tx, actor, c); err != nil {
			return err
		}
		var v model.CreationVersion
		if err := tx.Where("id = ? AND creation_id = ?", req.VersionID, id).First(&v).Error; err != nil {
			return err
		}
		if actor.Role == "guest" {
			if err := authorizeSharedPhotos(ctx, tx, actor, v); err != nil {
				return err
			}
		}
		err := tx.Where("creation_id = ?", id).First(&share).Error
		now := time.Now()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			share = model.CreationShare{ID: uuid.NewString(), CreationID: id, VersionID: v.ID, Enabled: true, CreatedAt: now, UpdatedAt: now}
			return tx.Create(&share).Error
		}
		if err != nil {
			return err
		}
		share.VersionID = v.ID
		share.Enabled = true
		share.UpdatedAt = now
		return tx.Model(&share).Updates(map[string]interface{}{"version_id": v.ID, "enabled": true, "updated_at": now}).Error
	})
	return share, err
}
func (s *CreationService) RevokeShare(ctx context.Context, actor CreationActor, id string) error {
	if !actor.valid() {
		return ErrCreationPhotoForbidden
	}
	if !s.sharingEnabled {
		return ErrCreationSharingUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c model.Creation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, actor.ID).First(&c).Error; err != nil {
			return err
		}
		return tx.Model(&model.CreationShare{}).Where("creation_id = ?", id).Updates(map[string]interface{}{"enabled": false, "updated_at": time.Now()}).Error
	})
}
func authorizeSharedPhotos(ctx context.Context, db *gorm.DB, actor CreationActor, v model.CreationVersion) error {
	if !actor.valid() {
		return ErrCreationPhotoForbidden
	}
	if actor.Role == "admin" {
		return nil
	}
	var ids []int64
	if json.Unmarshal([]byte(v.UsedPhotoIDs), &ids) != nil || len(ids) == 0 {
		return ErrCreationPhotoForbidden
	}
	return validateCreationPhotos(ctx, db, actor, mergeCreationPhotos(ids))
}
func (s *CreationService) sharedVersion(ctx context.Context, actor CreationActor, id string) (model.CreationShare, model.Creation, model.CreationVersion, error) {
	var share model.CreationShare
	var c model.Creation
	var v model.CreationVersion
	if !actor.valid() {
		return share, c, v, ErrCreationPhotoForbidden
	}
	if !s.sharingEnabled {
		return share, c, v, ErrCreationSharingUnavailable
	}
	if err := s.db.WithContext(ctx).Where("id = ? AND enabled = ?", id, true).First(&share).Error; err != nil {
		return share, c, v, err
	}
	if err := s.db.WithContext(ctx).Where("id = ?", share.CreationID).First(&c).Error; err != nil {
		return share, c, v, err
	}
	if err := s.db.WithContext(ctx).Where("id = ? AND creation_id = ?", share.VersionID, share.CreationID).First(&v).Error; err != nil {
		return share, c, v, err
	}
	err := authorizeSharedPhotos(ctx, s.db, actor, v)
	return share, c, v, err
}
func (s *CreationService) Shared(ctx context.Context, actor CreationActor, id string) (SharedCreation, error) {
	share, c, v, err := s.sharedVersion(ctx, actor, id)
	if err != nil {
		return SharedCreation{}, err
	}
	return SharedCreation{ID: share.ID, Title: c.Title, VersionID: v.ID, Verified: v.Verified, CreatedAt: v.CreatedAt}, nil
}
func (s *CreationService) SharedPreview(ctx context.Context, actor CreationActor, id string) (string, error) {
	_, c, v, err := s.sharedVersion(ctx, actor, id)
	if err != nil {
		return "", err
	}
	return s.renderCreationVersion(v, c.PhotoIDs)
}
