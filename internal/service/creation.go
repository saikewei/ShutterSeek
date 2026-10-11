package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	htmlstd "html"
	"io"
	"log"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"shutterseek/internal/creationrpc"
	"shutterseek/internal/model"
)

var ErrCreationConflict = errors.New("creation is busy or base version changed")
var ErrCreationInvalid = errors.New("invalid creation request")

const CreationCSP = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; media-src 'none'; object-src 'none'; frame-src about:; base-uri 'none'; form-action 'none'; sandbox allow-scripts"
const creationHTMLLimit = 512 << 10

type CreationService struct {
	db         *gorm.DB
	thumbnails string
	conn       *grpc.ClientConn
	mutex      sync.Mutex
	cancel     map[string]context.CancelFunc
	slots      chan struct{}
	stop       context.CancelFunc
}
type CreationRequest struct {
	Title        string         `json:"title"`
	PhotoIDs     []int64        `json:"photo_ids"`
	AlbumID      int64          `json:"album_id"`
	UsedPhotoIDs []int64        `json:"used_photo_ids,omitempty"`
	Rotations    map[string]int `json:"rotations,omitempty"`
}
type CreationProposal struct {
	Structure    string  `json:"structure"`
	Style        string  `json:"style"`
	UsedPhotoIDs []int64 `json:"used_photo_ids"`
}
type CreationRunView struct {
	model.CreationRun
	Proposal      *CreationProposal `json:"proposal,omitempty"`
	PhotoRevision *int              `json:"photo_revision,omitempty"`
}
type CreationRunRequest struct {
	Proposal      *CreationProposal `json:"proposal,omitempty"`
	Action        string            `json:"action"`
	Message       string            `json:"message"`
	BaseVersion   string            `json:"base_version"`
	PhotoRevision *int              `json:"photo_revision,omitempty"`
}

func jsonText(v interface{}) string { b, _ := json.Marshal(v); return string(b) }
func mapText(s string) map[string]interface{} {
	out := map[string]interface{}{}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}
func credentialHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// StartCreationService is called only when the explicit feature flag is enabled.
func StartCreationService(db *gorm.DB, thumbnails, agentAddress, listenAddress string) (*CreationService, error) {
	conn, err := grpc.NewClient(agentAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(creationrpc.MaxMessage), grpc.MaxCallSendMsgSize(creationrpc.MaxMessage)))
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		conn.Close()
		return nil, err
	}
	s := &CreationService{db: db, thumbnails: thumbnails, conn: conn, cancel: map[string]context.CancelFunc{}, slots: make(chan struct{}, 5)}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(creationrpc.MaxMessage), grpc.MaxSendMsgSize(creationrpc.MaxMessage))
	creationrpc.RegisterWorkspace(server, s)
	ctx, cancel := context.WithCancel(context.Background())
	s.stop = func() { cancel(); server.Stop(); conn.Close() }
	go server.Serve(listener)
	go s.worker(ctx)
	return s, nil
}
func (s *CreationService) Close() {
	if s != nil {
		s.stop()
		s.mutex.Lock()
		defer s.mutex.Unlock()
		for _, cancel := range s.cancel {
			cancel()
		}
	}
}
func (s *CreationService) Owned(ctx context.Context, user int64, id string) (model.Creation, error) {
	var c model.Creation
	err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, user).First(&c).Error
	return c, err
}
func (s *CreationService) List(ctx context.Context, user int64) ([]model.Creation, error) {
	out := []model.Creation{}
	err := s.db.WithContext(ctx).Where("user_id = ?", user).Order("created_at DESC").Limit(100).Find(&out).Error
	return out, err
}
func (s *CreationService) Create(ctx context.Context, user int64, req CreationRequest) (model.Creation, error) {
	var c model.Creation
	title, err := normalizedCreationTitle(req.Title, true)
	if err != nil {
		return c, err
	}
	req.Title = title
	if len(req.PhotoIDs) > 200 || (req.AlbumID > 0 && len(req.PhotoIDs) > 0) {
		return c, ErrCreationInvalid
	}
	ids := req.PhotoIDs
	if req.AlbumID > 0 {
		if err := s.db.WithContext(ctx).Table("album_photos").Where("album_id = ?", req.AlbumID).Order("photo_id").Limit(201).Pluck("photo_id", &ids).Error; err != nil {
			return c, err
		}
	}
	if len(ids) == 0 || len(ids) > 200 {
		return c, ErrCreationInvalid
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return c, ErrCreationInvalid
		}
		seen[id] = true
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Photo{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
		return c, err
	}
	if count != int64(len(ids)) {
		return c, ErrCreationInvalid
	}
	state := "{}"
	if req.UsedPhotoIDs != nil || req.Rotations != nil {
		selected := req.UsedPhotoIDs
		if selected == nil {
			selected = ids
		}
		settings, err := creationPhotoSettings(ids, selected, req.Rotations)
		if err != nil {
			return c, err
		}
		settings["photo_revision"] = 0
		state = jsonText(settings)
	}
	now := time.Now()
	c = model.Creation{ID: uuid.NewString(), UserID: user, Title: req.Title, PhotoIDs: jsonText(ids), State: state, CreatedAt: now, UpdatedAt: now}
	return c, s.db.WithContext(ctx).Create(&c).Error
}
func (s *CreationService) Detail(ctx context.Context, user int64, id string) (map[string]interface{}, error) {
	c, err := s.Owned(ctx, user, id)
	if err != nil {
		return nil, err
	}
	messages := []model.CreationMessage{}
	versions := []model.CreationVersion{}
	runs := []model.CreationRun{}
	if err = s.db.WithContext(ctx).Where("creation_id = ?", id).Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	if err = s.db.WithContext(ctx).Where("creation_id = ?", id).Order("created_at DESC").Omit("html").Find(&versions).Error; err != nil {
		return nil, err
	}
	if err = s.db.WithContext(ctx).Where("creation_id = ?", id).Order("created_at DESC").Limit(20).Find(&runs).Error; err != nil {
		return nil, err
	}
	var ids []int64
	_ = json.Unmarshal([]byte(c.PhotoIDs), &ids)
	views := make([]CreationRunView, 0, len(runs))
	for _, run := range runs {
		view := CreationRunView{CreationRun: run}
		if run.Action == "confirm" {
			var input CreationRunRequest
			if json.Unmarshal([]byte(run.Input), &input) == nil {
				view.Proposal = input.Proposal
				view.PhotoRevision = input.PhotoRevision
			}
		}
		views = append(views, view)
	}
	return map[string]interface{}{"creation": c, "photo_ids": ids, "state": mapText(c.State), "messages": messages, "versions": versions, "runs": views}, nil
}
func (s *CreationService) Run(ctx context.Context, user int64, id string, req CreationRunRequest) (model.CreationRun, error) {
	var run model.CreationRun
	if req.Action != "answer" && req.Action != "skip" && req.Action != "confirm" && req.Action != "modify" && req.Action != "localize" {
		return run, ErrCreationInvalid
	}
	if len(req.Message) > 8000 {
		return run, ErrCreationInvalid
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(0x53534352454154)).Error; err != nil {
			return err
		}
		var c model.Creation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, user).First(&c).Error; err != nil {
			return err
		}
		if c.CurrentVersion != req.BaseVersion {
			return ErrCreationConflict
		}
		state := mapText(c.State)
		if req.PhotoRevision != nil && *req.PhotoRevision != photoRevision(state) {
			return ErrCreationConflict
		}
		if req.Action == "modify" && c.CurrentVersion == "" {
			return ErrCreationInvalid
		}
		if req.Proposal != nil {
			if req.Action != "confirm" || req.Proposal.Structure == "" || req.Proposal.Style == "" || len(req.Proposal.Structure) > 8000 || len(req.Proposal.Style) > 2000 || len(req.Proposal.UsedPhotoIDs) == 0 || len(req.Proposal.UsedPhotoIDs) > 200 {
				return ErrCreationInvalid
			}
			var scope []int64
			if err := json.Unmarshal([]byte(c.PhotoIDs), &scope); err != nil {
				return err
			}
			allowed := map[int64]bool{}
			for _, id := range scope {
				allowed[id] = true
			}
			seen := map[int64]bool{}
			for _, id := range req.Proposal.UsedPhotoIDs {
				if !allowed[id] || seen[id] {
					return ErrCreationInvalid
				}
				seen[id] = true
			}
		}
		if req.Action == "confirm" && state["proposal"] == nil {
			return ErrCreationInvalid
		}
		var count int64
		if err := tx.Model(&model.CreationRun{}).Where("(user_id = ? OR creation_id = ?) AND status IN ?", user, id, []string{"queued", "running"}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrCreationConflict
		}
		if err := tx.Model(&model.CreationRun{}).Where("status IN ?", []string{"queued", "running"}).Count(&count).Error; err != nil {
			return err
		}
		if count >= 5 {
			return ErrCreationConflict
		}
		now := time.Now()
		run = model.CreationRun{ID: uuid.NewString(), CreationID: id, UserID: user, BaseVersion: req.BaseVersion, Action: req.Action, Input: jsonText(req), Status: "queued", ExpiresAt: now, Report: "{}", CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		return tx.Create(&model.CreationMessage{ID: uuid.NewString(), CreationID: id, Role: "user", Content: req.Message, CreatedAt: now}).Error
	})
	return run, err
}
func (s *CreationService) RunOwned(ctx context.Context, user int64, creation, runID string) (model.CreationRun, error) {
	var r model.CreationRun
	err := s.db.WithContext(ctx).Where("id = ? AND creation_id = ? AND user_id = ?", runID, creation, user).First(&r).Error
	return r, err
}
func (s *CreationService) Cancel(ctx context.Context, user int64, creation, runID string) error {
	if _, err := s.RunOwned(ctx, user, creation, runID); err != nil {
		return err
	}
	s.mutex.Lock()
	if cancel := s.cancel[runID]; cancel != nil {
		cancel()
	}
	s.mutex.Unlock()
	return s.db.WithContext(ctx).Model(&model.CreationRun{}).Where("id = ? AND status IN ?", runID, []string{"queued", "running"}).Updates(map[string]interface{}{"status": "cancelled", "credential_hash": "", "updated_at": time.Now()}).Error
}
func (s *CreationService) Events(ctx context.Context, user int64, creation, runID string, after int64) ([]model.CreationEvent, string, error) {
	r, err := s.RunOwned(ctx, user, creation, runID)
	if err != nil {
		return nil, "", err
	}
	events := []model.CreationEvent{}
	err = s.db.WithContext(ctx).Where("run_id = ? AND id > ?", runID, after).Order("id").Limit(100).Find(&events).Error
	return events, r.Status, err
}
func (s *CreationService) Restore(ctx context.Context, user int64, id, version, base string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c model.Creation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, user).First(&c).Error; err != nil {
			return err
		}
		if c.CurrentVersion != base {
			return ErrCreationConflict
		}
		var count int64
		if err := tx.Model(&model.CreationRun{}).Where("creation_id = ? AND status IN ?", id, []string{"queued", "running"}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrCreationConflict
		}
		var v model.CreationVersion
		if err := tx.Where("id = ? AND creation_id = ?", version, id).First(&v).Error; err != nil {
			return err
		}
		restored := mapText(v.State)
		var scope []int64
		_ = json.Unmarshal([]byte(jsonText(restored["photo_ids"])), &scope)
		if len(scope) == 0 {
			_ = json.Unmarshal([]byte(v.UsedPhotoIDs), &scope)
		}
		updates := map[string]interface{}{"current_version": version, "state": v.State, "updated_at": time.Now()}
		if len(scope) > 0 {
			restored["photo_ids"] = scope
			restored["photo_revision"] = photoRevision(mapText(c.State)) + 1
			restored["photos_changed"] = false
			updates["photo_ids"] = jsonText(scope)
			updates["state"] = jsonText(restored)
		}
		return tx.Model(&c).Updates(updates).Error
	})
}

var photoPlaceholder = regexp.MustCompile(`ss-photo:([0-9]+)`)

// Preview embeds only the authorized work's thumbnails; generated code has no network access.
func (s *CreationService) Preview(ctx context.Context, user int64, id, version string) (string, error) {
	c, err := s.Owned(ctx, user, id)
	if err != nil {
		return "", err
	}
	if version == "" {
		version = c.CurrentVersion
	}
	var v model.CreationVersion
	if err = s.db.WithContext(ctx).Where("id = ? AND creation_id = ?", version, id).First(&v).Error; err != nil {
		return "", err
	}
	var ids []int64
	_ = json.Unmarshal([]byte(v.UsedPhotoIDs), &ids)
	if len(ids) == 0 {
		_ = json.Unmarshal([]byte(c.PhotoIDs), &ids)
	}
	rotations := photoRotations(mapText(v.State))
	assets := map[string]string{}
	wanted := map[string]bool{}
	for _, match := range photoPlaceholder.FindAllStringSubmatch(v.HTML, -1) {
		wanted[match[1]] = true
	}
	for _, id := range ids {
		if !wanted[fmt.Sprint(id)] {
			continue
		}
		b, mime, err := s.creationPhotoBytes(id, rotations[fmt.Sprint(id)])
		if err != nil {
			continue
		}
		assets[fmt.Sprint(id)] = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
	}
	expandedSize := len(v.HTML)
	for _, match := range photoPlaceholder.FindAllStringSubmatch(v.HTML, -1) {
		expandedSize += len(assets[match[1]])
		if expandedSize > 64<<20 {
			return "", errors.New("expanded page exceeds asset budget")
		}
	}
	html := photoPlaceholder.ReplaceAllStringFunc(v.HTML, func(token string) string { return assets[strings.TrimPrefix(token, "ss-photo:")] })
	// A response CSP is authoritative even if generated HTML attempts to remove its meta tag.
	return CreationPreviewDocument(html), nil
}

// CreationPreviewDocument keeps the generated document in a nested opaque
// frame. The trusted parent's frame-src policy also blocks self-navigation,
// which the child's default-src directive does not control.
func CreationPreviewDocument(source string) string {
	contentPolicy := strings.Replace(CreationCSP, "frame-src about:", "frame-src 'none'", 1)
	contentPolicy = strings.Replace(contentPolicy, "; sandbox allow-scripts", "", 1)
	content := "<meta http-equiv=\"Content-Security-Policy\" content=\"" + htmlstd.EscapeString(contentPolicy) + "\">"
	end := strings.Index(source, ">")
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(source)), "<!doctype") && end >= 0 {
		source = source[:end+1] + content + source[end+1:]
	} else {
		source = "<!doctype html>" + content + source
	}
	return "<!doctype html><html><head><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><style>html,body{margin:0;overflow:hidden;height:100%}iframe{border:0;width:100%;height:100%;display:block}</style></head><body><iframe sandbox=\"allow-scripts\" referrerpolicy=\"no-referrer\" title=\"作品内容\" srcdoc=\"" + htmlstd.EscapeString(source) + "\"></iframe></body></html>"
}
func (s *CreationService) thumbnail(id int64) ([]byte, error) {
	f, err := os.Open(filepath.Join(s.thumbnails, fmt.Sprintf("%d.webp", id)))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if len(b) > 2<<20 {
		return nil, errors.New("photo asset exceeds budget")
	}
	return b, err
}
func (s *CreationService) Screenshot(ctx context.Context, user int64, id, assetID string) ([]byte, error) {
	if _, err := s.Owned(ctx, user, id); err != nil {
		return nil, err
	}
	var a model.CreationAsset
	err := s.db.WithContext(ctx).Joins("JOIN creation_runs ON creation_runs.id = creation_assets.run_id").Where("creation_assets.id = ? AND creation_runs.creation_id = ?", assetID, id).First(&a).Error
	return a.Data, err
}

// Empty workspaces must remain read-only during startup and idle cleanup.
func (s *CreationService) recoverCreationRuns(ctx context.Context) error {
	var ids []string
	if err := s.db.WithContext(ctx).Model(&model.CreationRun{}).Where("status = ?", "running").Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(&model.CreationRun{}).Where("id IN ? AND status = ?", ids, "running").Updates(map[string]interface{}{"status": "queued", "credential_hash": ""}).Error
}
func (s *CreationService) cleanupCreationAssets(ctx context.Context) error {
	var ids []string
	if err := s.db.WithContext(ctx).Model(&model.CreationAsset{}).
		Joins("JOIN creation_runs ON creation_runs.id = creation_assets.run_id").
		Where("creation_runs.status IN ?", []string{"failed", "cancelled"}).
		Limit(256).Pluck("creation_assets.id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Where("id IN ?", ids).Delete(&model.CreationAsset{}).Error
}
func (s *CreationService) worker(ctx context.Context) {
	// Restarted runs keep their checkpoints; credentials are reissued, never restored.
	_ = s.recoverCreationRuns(ctx)
	cleanupAt := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if time.Since(cleanupAt) > time.Minute {
			_ = s.cleanupCreationAssets(ctx)
			cleanupAt = time.Now()
		}
		var runs []model.CreationRun
		if err := s.db.WithContext(ctx).Where("status = ?", "queued").Order("created_at,id").Limit(5).Find(&runs).Error; err != nil {
			continue
		}
		for _, r := range runs {
			s.mutex.Lock()
			_, exists := s.cancel[r.ID]
			if exists {
				s.mutex.Unlock()
				continue
			}
			select {
			case s.slots <- struct{}{}:
			default:
				s.mutex.Unlock()
				continue
			}
			runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			s.cancel[r.ID] = cancel
			s.mutex.Unlock()
			go func(r model.CreationRun) {
				defer func() { cancel(); <-s.slots; s.mutex.Lock(); delete(s.cancel, r.ID); s.mutex.Unlock() }()
				s.execute(runCtx, r)
			}(r)
		}
	}
}
func (s *CreationService) execute(ctx context.Context, r model.CreationRun) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	result := s.db.WithContext(ctx).Model(&model.CreationRun{}).Where("id = ? AND status = ?", r.ID, "queued").Updates(map[string]interface{}{"status": "running", "credential_hash": credentialHash(token), "expires_at": time.Now().Add(31 * time.Minute), "updated_at": time.Now()})
	if result.Error != nil || result.RowsAffected != 1 {
		return
	}
	s.db.WithContext(ctx).Create(&model.CreationEvent{RunID: r.ID, Data: jsonText(map[string]interface{}{"stage": "started", "queue_seconds": time.Since(r.CreatedAt).Seconds()}), CreatedAt: time.Now()})
	failureReport := map[string]interface{}{"error": "创作任务失败，请重试；已保存的方案和作品会保留。"}
	err := creationrpc.Execute(ctx, s.conn, map[string]interface{}{"run_id": r.ID, "credential": token}, func(event map[string]interface{}) error {
		if event["stage"] == "failed" {
			code, _ := event["error_code"].(string)
			failureReport = creationFailureReport(code)
		}
		return s.db.WithContext(ctx).Create(&model.CreationEvent{RunID: r.ID, Data: jsonText(event), CreatedAt: time.Now()}).Error
	})
	if ctx.Err() != nil && s.stop != nil {
		// On process shutdown recovery must remain possible; timeout and explicit cancellation terminate.
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}
	}
	if status.Code(err) == codes.Unavailable && time.Since(r.CreatedAt) < 30*time.Minute {
		s.db.Model(&model.CreationRun{}).Where("id = ? AND status = ?", r.ID, "running").Updates(map[string]interface{}{"status": "queued", "credential_hash": "", "updated_at": time.Now()})
		return
	}
	if err != nil && !errors.Is(err, io.EOF) {
		log.Printf("creation: run=%s failed rpc_code=%s error_type=%T", r.ID, status.Code(err), err)
		s.db.Model(&model.CreationRun{}).Where("id = ? AND status = ?", r.ID, "running").Updates(map[string]interface{}{"status": "failed", "credential_hash": "", "report": jsonText(failureReport), "updated_at": time.Now()})
	} else {
		s.db.Model(&model.CreationRun{}).Where("id = ? AND status = ?", r.ID, "running").Updates(map[string]interface{}{"status": "failed", "credential_hash": "", "updated_at": time.Now()})
	}
}
func creationFailureReport(code string) map[string]interface{} {
	messages := map[string]string{
		"candidate_html_invalid":         "模型返回的网页内容无效或过大。请重试生成。",
		"candidate_embedded_image":       "模型返回的网页包含内嵌图片数据，自动纠正未成功。请重试生成。",
		"candidate_restricted_material":  "模型返回的网页包含不允许的凭证相关内容，自动纠正未成功。请重试生成。",
		"checkpoint_image_material":      "任务内容含不允许写入检查点的图片数据，已保存的方案和作品会保留。",
		"checkpoint_credential_material": "任务内容触发了检查点的凭证保护，已保存的方案和作品会保留。",
		"photo_selection_invalid":        "模型返回的照片列表无效。请重试生成。",
		"photo_references_mismatch":      "模型返回的网页与照片列表不一致。自动纠正未成功，请重试生成。",
		"confirmed_photos_changed":       "模型未保留你确认的选图。自动纠正未成功，请重试生成，或调整选图后再次确认。",
		"interaction_manifest_invalid":   "模型返回的交互信息无效。请重试生成。",
	}
	message, ok := messages[code]
	if !ok {
		code = "agent_execution_failed"
		message = "创作任务失败，请重试；已保存的方案和作品会保留。"
	}
	return map[string]interface{}{"error_code": code, "error": message}
}

func (s *CreationService) Call(ctx context.Context, in *structpb.Struct) (*structpb.Struct, error) {
	data := in.AsMap()
	runID, _ := data["run_id"].(string)
	token, _ := data["credential"].(string)
	op, _ := data["op"].(string)
	var r model.CreationRun
	if token == "" || s.db.WithContext(ctx).Where("id = ? AND credential_hash = ? AND expires_at > ? AND status = ?", runID, credentialHash(token), time.Now(), "running").First(&r).Error != nil {
		return nil, status.Error(codes.PermissionDenied, "invalid run credential")
	}
	var c model.Creation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", r.CreationID, r.UserID).First(&c).Error; err != nil {
		return nil, status.Error(codes.PermissionDenied, "work unavailable")
	}
	payload, _ := data["payload"].(map[string]interface{})
	if payload == nil {
		payload = map[string]interface{}{}
	}
	out, err := s.callback(ctx, r, c, op, payload)
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	// Normalize typed slices, model structs, timestamps, and JSON tags for Struct.
	encoded, err := json.Marshal(out)
	if err == nil {
		response := new(structpb.Struct)
		err = protojson.Unmarshal(encoded, response)
		if err == nil {
			return response, nil
		}
	}
	log.Printf("creation: callback serialization failed run=%s op=%s error_type=%T", r.ID, op, err)
	return nil, status.Error(codes.Internal, "workspace response serialization failed")
}
func (s *CreationService) callback(ctx context.Context, r model.CreationRun, c model.Creation, op string, p map[string]interface{}) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	switch op {
	case "context":
		var ids []int64
		_ = json.Unmarshal([]byte(c.PhotoIDs), &ids)
		out = map[string]interface{}{"creation_id": c.ID, "thread_id": c.ID, "photo_ids": ids, "state": mapText(c.State), "input": mapText(r.Input), "title": c.Title, "base_version": r.BaseVersion}
		messages := []model.CreationMessage{}
		if err := s.db.WithContext(ctx).Where("creation_id = ?", c.ID).Order("created_at DESC").Limit(40).Find(&messages).Error; err != nil {
			return nil, err
		}
		sort.Slice(messages, func(i, j int) bool { return messages[i].CreatedAt.Before(messages[j].CreatedAt) })
		out["messages"] = messages
		if c.CurrentVersion != "" {
			var v model.CreationVersion
			if err := s.db.WithContext(ctx).Where("id = ? AND creation_id = ?", c.CurrentVersion, c.ID).First(&v).Error; err != nil {
				return nil, err
			}
			out["html"] = v.HTML
		}
	case "photo":
		n, ok := p["id"].(float64)
		if !ok || n != float64(int64(n)) {
			return nil, ErrCreationInvalid
		}
		var ids []int64
		_ = json.Unmarshal([]byte(c.PhotoIDs), &ids)
		allowed := false
		for _, id := range ids {
			if id == int64(n) {
				allowed = true
			}
		}
		if !allowed {
			return nil, errors.New("photo outside work scope")
		}
		angle := photoRotations(mapText(c.State))[fmt.Sprint(int64(n))]
		b, mime, err := s.creationPhotoBytes(int64(n), angle)
		if err != nil {
			return nil, errors.New("photo asset unavailable")
		}
		out["data"] = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
		out["rotation"] = angle
		out["width"], out["height"] = creationAssetDimensions(b)
	case "checkpoint_get":
		var checkpoint model.CreationCheckpoint
		err := s.db.WithContext(ctx).Where("run_id = ?", r.ID).Order("updated_at DESC").First(&checkpoint).Error
		if err == nil {
			out["checkpoint"] = mapText(checkpoint.Payload)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	case "checkpoint_put":
		if len(jsonText(p)) > 2<<20 {
			return nil, ErrCreationInvalid
		}
		// State stores references only; image data and credentials must never enter checkpoints.
		text := jsonText(p)
		if strings.Contains(text, "data:image/") || strings.Contains(text, "credential") {
			return nil, ErrCreationInvalid
		}
		row := model.CreationCheckpoint{ID: r.ID, RunID: r.ID, Payload: text, UpdatedAt: time.Now()}
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_at"})}).Create(&row).Error; err != nil {
			return nil, err
		}
	case "asset":
		encoded, _ := p["data"].(string)
		b, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(b) > 6<<20 || len(b) < 8 || string(b[:8]) != "\x89PNG\r\n\x1a\n" {
			return nil, ErrCreationInvalid
		}
		var count int64
		s.db.WithContext(ctx).Model(&model.CreationAsset{}).Where("run_id = ?", r.ID).Count(&count)
		if count >= 24 {
			var existing int64
			if err := s.db.WithContext(ctx).Model(&model.CreationAsset{}).Where("id = ? AND run_id = ?", credentialHash(r.ID+jsonText(p["metadata"])), r.ID).Count(&existing).Error; err != nil {
				return nil, err
			}
			if existing == 0 {
				return nil, ErrCreationInvalid
			}
		}
		row := model.CreationAsset{ID: credentialHash(r.ID + jsonText(p["metadata"])), RunID: r.ID, Data: b, Metadata: jsonText(p["metadata"]), CreatedAt: time.Now()}
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"data", "metadata"})}).Create(&row).Error; err != nil {
			return nil, err
		}
		out["id"] = row.ID
	case "asset_get":
		id, _ := p["id"].(string)
		var asset model.CreationAsset
		if err := s.db.WithContext(ctx).Where("id = ? AND run_id = ?", id, r.ID).First(&asset).Error; err != nil {
			return nil, err
		}
		out["data"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(asset.Data)
	case "finish":
		state, _ := p["state"].(map[string]interface{})
		report, _ := p["report"].(map[string]interface{})
		if state == nil || strings.Contains(jsonText(state), "data:image/") || len(jsonText(state)) > 2<<20 {
			return nil, ErrCreationInvalid
		}
		html, _ := p["html"].(string)
		verified, _ := p["verified"].(bool)
		if len(html) > creationHTMLLimit {
			return nil, ErrCreationInvalid
		}
		used := p["used_photo_ids"]
		var usedIDs []int64
		_ = json.Unmarshal([]byte(jsonText(used)), &usedIDs)
		var scope []int64
		_ = json.Unmarshal([]byte(c.PhotoIDs), &scope)
		allowed := map[int64]bool{}
		for _, id := range scope {
			allowed[id] = true
		}
		for _, id := range usedIDs {
			if !allowed[id] {
				return nil, ErrCreationInvalid
			}
		}
		for _, token := range photoPlaceholder.FindAllStringSubmatch(html, -1) {
			if !allowedID(token[1], allowed) {
				return nil, ErrCreationInvalid
			}
		}
		if html != "" {
			sealPhotoSnapshot(state, c, usedIDs)
		}
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var current model.Creation
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", c.ID).First(&current).Error; err != nil {
				return err
			}
			if current.CurrentVersion != r.BaseVersion {
				return ErrCreationConflict
			}
			var active model.CreationRun
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", r.ID, "running").First(&active).Error; err != nil {
				return err
			}
			updates := map[string]interface{}{"state": jsonText(state), "updated_at": time.Now()}
			nextStatus := "waiting"
			if html != "" {
				if verified && !validCreationReport(report) {
					return errors.New("visual verification evidence is missing")
				}
				v := model.CreationVersion{ID: uuid.NewString(), CreationID: c.ID, RunID: r.ID, HTML: html, State: jsonText(state), Report: jsonText(report), UsedPhotoIDs: jsonText(usedIDs), Verified: verified, CreatedAt: time.Now()}
				if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "run_id"}}, DoNothing: true}).Create(&v).Error; err != nil {
					return err
				}
				if err := tx.Where("run_id = ?", r.ID).First(&v).Error; err != nil {
					return err
				}
				updates["current_version"] = v.ID
				nextStatus = "completed"
				out["version_id"] = v.ID
				// Keep only the final round's screenshots for this version.
				refs := []string{}
				if items, ok := report["screenshots"].([]interface{}); ok {
					for _, item := range items {
						if a, ok := item.(map[string]interface{}); ok {
							if id, ok := a["id"].(string); ok {
								refs = append(refs, id)
							}
						}
					}
				}
				if len(refs) > 0 {
					var assetCount int64
					if err := tx.Model(&model.CreationAsset{}).Where("run_id = ? AND id IN ?", r.ID, refs).Count(&assetCount).Error; err != nil {
						return err
					}
					if assetCount != int64(len(refs)) {
						return errors.New("screenshot reference is outside run scope")
					}
					if err := tx.Where("run_id = ? AND id NOT IN ?", r.ID, refs).Delete(&model.CreationAsset{}).Error; err != nil {
						return err
					}
				} else {
					if err := tx.Where("run_id = ?", r.ID).Delete(&model.CreationAsset{}).Error; err != nil {
						return err
					}
				}
			}
			if err := tx.Model(&current).Updates(updates).Error; err != nil {
				return err
			}
			if err := tx.Model(&active).Updates(map[string]interface{}{"status": nextStatus, "report": jsonText(report), "credential_hash": "", "updated_at": time.Now()}).Error; err != nil {
				return err
			}
			message, _ := p["message"].(string)
			return tx.Create(&model.CreationMessage{ID: uuid.NewString(), CreationID: c.ID, Role: "assistant", Content: message, CreatedAt: time.Now()}).Error
		})
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrCreationInvalid
	}
	return out, nil
}
func allowedID(text string, scope map[int64]bool) bool {
	var id int64
	_, err := fmt.Sscan(text, &id)
	return err == nil && scope[id]
}
func validCreationReport(report map[string]interface{}) bool {
	rounds, ok := report["rounds"].(float64)
	if !ok || rounds < 1 || rounds > 3 || math.Trunc(rounds) != rounds {
		return false
	}
	screenshots, ok := report["screenshots"].([]interface{})
	if !ok || len(screenshots) < 2 || len(screenshots) > 8 {
		return false
	}
	views := map[string]bool{}
	for _, item := range screenshots {
		shot, ok := item.(map[string]interface{})
		if !ok {
			return false
		}
		id, _ := shot["id"].(string)
		if id == "" {
			return false
		}
		name, _ := shot["viewport"].(string)
		views[name] = true
	}
	if !views["desktop"] || !views["mobile"] {
		return false
	}
	issues, ok := report["issues"].([]interface{})
	if !ok {
		return false
	}
	for _, item := range issues {
		issue, ok := item.(map[string]interface{})
		if !ok || issue["severity"] == "blocking" {
			return false
		}
	}
	return report["browser_ok"] == true && report["visual_reviewed"] == true
}
