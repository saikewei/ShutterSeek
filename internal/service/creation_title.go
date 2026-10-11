package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"shutterseek/internal/model"
)

type CreationTitleRequest struct {
	Title     string  `json:"title"`
	BaseTitle *string `json:"base_title"`
}

func normalizedCreationTitle(title string, allowDefault bool) (string, error) {
	title = strings.TrimSpace(title)
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) > 100 {
		return "", ErrCreationInvalid
	}
	if title == "" {
		if !allowDefault {
			return "", ErrCreationInvalid
		}
		title = "照片作品"
	}
	return title, nil
}

// Rename changes metadata only, preserving the running job and all photo/version state.
func (s *CreationService) Rename(ctx context.Context, actor CreationActor, id string, req CreationTitleRequest) error {
	if !actor.valid() {
		return ErrCreationPhotoForbidden
	}
	title, err := normalizedCreationTitle(req.Title, false)
	if err != nil || req.BaseTitle == nil {
		return ErrCreationInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c model.Creation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, actor.ID).First(&c).Error; err != nil {
			return err
		}
		if c.Title != *req.BaseTitle {
			return ErrCreationConflict
		}
		return tx.Model(&c).Updates(map[string]interface{}{"title": title, "updated_at": time.Now()}).Error
	})
}
