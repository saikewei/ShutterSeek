package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"image"
	"image/jpeg"
	"net/http"
	"shutterseek/internal/model"
	"strconv"
	"time"
)

type CreationPhotosRequest struct {
	Proposal     *CreationProposal `json:"proposal,omitempty"`
	PhotoIDs     []int64           `json:"photo_ids"`
	UsedPhotoIDs []int64           `json:"used_photo_ids"`
	Rotations    map[string]int    `json:"rotations"`
	BaseVersion  string            `json:"base_version"`
	BaseRevision int               `json:"base_revision"`
}

func creationPhotoSettings(ids, selected []int64, rotations map[string]int) (map[string]interface{}, error) {
	if len(ids) == 0 || len(ids) > 200 || len(selected) == 0 || len(selected) > 200 {
		return nil, ErrCreationInvalid
	}
	scope := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || scope[id] {
			return nil, ErrCreationInvalid
		}
		scope[id] = true
	}
	chosen := map[int64]bool{}
	for _, id := range selected {
		if !scope[id] || chosen[id] {
			return nil, ErrCreationInvalid
		}
		chosen[id] = true
	}
	edits := map[string]int{}
	for key, angle := range rotations {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || fmt.Sprint(id) != key || !scope[id] || (angle != 0 && angle != 90 && angle != 180 && angle != 270) {
			return nil, ErrCreationInvalid
		}
		if angle != 0 {
			edits[key] = angle
		}
	}
	return map[string]interface{}{"photo_ids": ids, "selected_photo_ids": selected, "photo_edits": edits}, nil
}
func photoRevision(state map[string]interface{}) int {
	var revision int
	_ = json.Unmarshal([]byte(jsonText(state["photo_revision"])), &revision)
	return revision
}
func photoRotations(state map[string]interface{}) map[string]int {
	out := map[string]int{}
	_ = json.Unmarshal([]byte(jsonText(state["photo_edits"])), &out)
	return out
}

// UpdatePhotos changes only work-local references and transforms.
func (s *CreationService) UpdatePhotos(ctx context.Context, user int64, id string, req CreationPhotosRequest) error {
	settings, err := creationPhotoSettings(req.PhotoIDs, req.UsedPhotoIDs, req.Rotations)
	if err != nil {
		return err
	}
	if req.Proposal != nil {
		if req.Proposal.Structure == "" || req.Proposal.Style == "" || len(req.Proposal.Structure) > 8000 || len(req.Proposal.Style) > 2000 {
			return ErrCreationInvalid
		}
		if _, err := creationPhotoSettings(req.PhotoIDs, req.Proposal.UsedPhotoIDs, req.Rotations); err != nil {
			return err
		}
		if jsonText(req.Proposal.UsedPhotoIDs) != jsonText(req.UsedPhotoIDs) {
			return ErrCreationInvalid
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c model.Creation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, user).First(&c).Error; err != nil {
			return err
		}
		state := mapText(c.State)
		if c.CurrentVersion != req.BaseVersion || photoRevision(state) != req.BaseRevision {
			return ErrCreationConflict
		}
		var active int64
		if err := tx.Model(&model.CreationRun{}).Where("creation_id = ? AND status IN ?", id, []string{"queued", "running"}).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return ErrCreationConflict
		}
		var count int64
		if err := tx.Model(&model.Photo{}).Where("id IN ?", req.PhotoIDs).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(req.PhotoIDs)) {
			return ErrCreationInvalid
		}
		previous := photoRotations(state)
		scope := map[int64]bool{}
		for _, pid := range req.PhotoIDs {
			scope[pid] = true
		}
		observations := []interface{}{}
		if records, ok := state["observations"].([]interface{}); ok {
			for _, record := range records {
				item, ok := record.(map[string]interface{})
				if !ok {
					continue
				}
				pid, ok := item["photo_id"].(float64)
				if !ok {
					continue
				}
				key := fmt.Sprint(int64(pid))
				if scope[int64(pid)] && previous[key] == req.Rotations[key] {
					observations = append(observations, item)
				}
			}
		}
		for key, value := range settings {
			state[key] = value
		}
		state["observations"] = observations
		state["photo_revision"] = req.BaseRevision + 1
		state["photos_changed"] = c.CurrentVersion != ""
		if req.Proposal != nil {
			state["proposal"] = req.Proposal
		} else if proposal, ok := state["proposal"].(map[string]interface{}); ok {
			proposal["used_photo_ids"] = req.UsedPhotoIDs
		}
		return tx.Model(&c).Updates(map[string]interface{}{"photo_ids": jsonText(req.PhotoIDs), "state": jsonText(state), "updated_at": time.Now()}).Error
	})
}
func (s *CreationService) PhotoContent(ctx context.Context, user int64, id string, photo int64) ([]byte, string, error) {
	c, err := s.Owned(ctx, user, id)
	if err != nil {
		return nil, "", err
	}
	var ids []int64
	_ = json.Unmarshal([]byte(c.PhotoIDs), &ids)
	for _, pid := range ids {
		if pid == photo {
			return s.creationPhotoBytes(photo, photoRotations(mapText(c.State))[fmt.Sprint(photo)])
		}
	}
	return nil, "", gorm.ErrRecordNotFound
}

// Rotation is encoded into the ephemeral asset shared by model review and preview.
func (s *CreationService) creationPhotoBytes(id int64, angle int) ([]byte, string, error) {
	b, err := s.thumbnail(id)
	if err != nil {
		return nil, "", err
	}
	if angle == 0 {
		return b, http.DetectContentType(b), nil
	}
	if angle != 90 && angle != 180 && angle != 270 {
		return nil, "", ErrCreationInvalid
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 {
		return nil, "", ErrCreationInvalid
	}
	source, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, "", err
	}
	bounds := source.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	dw, dh := w, h
	if angle == 90 || angle == 270 {
		dw, dh = h, w
	}
	result := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch angle {
			case 90:
				dx, dy = h-1-y, x
			case 180:
				dx, dy = w-1-x, h-1-y
			case 270:
				dx, dy = y, w-1-x
			}
			result.Set(dx, dy, source.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	var output bytes.Buffer
	if err := jpeg.Encode(&output, result, &jpeg.Options{Quality: 90}); err != nil {
		return nil, "", err
	}
	return output.Bytes(), "image/jpeg", nil
}

// sealPhotoSnapshot keeps scope and transforms authoritative on the Go side.
func sealPhotoSnapshot(state map[string]interface{}, c model.Creation, selected []int64) {
	var scope []int64
	_ = json.Unmarshal([]byte(c.PhotoIDs), &scope)
	state["photo_ids"] = scope
	state["selected_photo_ids"] = selected
	state["photo_edits"] = photoRotations(mapText(c.State))
	state["photo_revision"] = photoRevision(mapText(c.State))
	state["photos_changed"] = false
}

func creationAssetDimensions(b []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}
