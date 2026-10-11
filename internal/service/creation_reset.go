package service

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"shutterseek/internal/model"
)

type CreationResetRequest struct {
	BaseVersion  string `json:"base_version"`
	BaseRevision *int   `json:"base_revision"`
}

// Reset discards all generated context while retaining the work's photo settings.
func (s *CreationService) Reset(ctx context.Context, user int64, id string, req CreationResetRequest) error {
	if req.BaseRevision == nil || *req.BaseRevision < 0 {
		return ErrCreationInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c model.Creation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, user).First(&c).Error; err != nil {
			return err
		}
		state := mapText(c.State)
		if c.CurrentVersion != req.BaseVersion || photoRevision(state) != *req.BaseRevision {
			return ErrCreationConflict
		}
		var active int64
		if err := tx.Model(&model.CreationRun{}).Where("creation_id = ? AND status IN ?", id, []string{"queued", "running"}).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return ErrCreationConflict
		}
		var ids, selected []int64
		if err := json.Unmarshal([]byte(c.PhotoIDs), &ids); err != nil {
			return ErrCreationInvalid
		}
		_ = json.Unmarshal([]byte(jsonText(state["selected_photo_ids"])), &selected)
		if len(selected) == 0 {
			var proposal CreationProposal
			_ = json.Unmarshal([]byte(jsonText(state["proposal"])), &proposal)
			selected = proposal.UsedPhotoIDs
		}
		if len(selected) == 0 {
			selected = ids
		}
		clean, err := creationPhotoSettings(ids, selected, photoRotations(state))
		if err != nil {
			return err
		}
		// Advancing the revision rejects requests prepared before the reset.
		clean["photo_revision"] = *req.BaseRevision + 1

		// Delete run children before their parents; every delete is scoped to this work.
		runs := tx.Model(&model.CreationRun{}).Select("id").Where("creation_id = ?", id)
		for _, child := range []interface{}{&model.CreationCheckpoint{}, &model.CreationAsset{}, &model.CreationEvent{}} {
			if err := tx.Where("run_id IN (?)", runs).Delete(child).Error; err != nil {
				return err
			}
		}
		for _, child := range []interface{}{&model.CreationVersion{}, &model.CreationMessage{}, &model.CreationRun{}} {
			if err := tx.Where("creation_id = ?", id).Delete(child).Error; err != nil {
				return err
			}
		}
		return tx.Model(&c).Updates(map[string]interface{}{
			"current_version": "", "state": jsonText(clean), "updated_at": time.Now(),
		}).Error
	})
}
