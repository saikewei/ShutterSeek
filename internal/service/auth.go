package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"shutterseek/internal/model"
)

// Sentinel errors for auth service.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserExists         = errors.New("username already exists")
	ErrInviteInvalid      = errors.New("invite code invalid or expired")
	ErrInviteExpired      = errors.New("invite code expired")
	ErrPasswordTooShort   = errors.New("password must be at least 6 characters")
	ErrTokenRevoked       = errors.New("token revoked")
	ErrInvalidRole        = errors.New("invalid role")
)

// Claims is the JWT payload.
type Claims struct {
	UserID int64  `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// AuthService handles authentication and invite codes.
type AuthService struct {
	DB        *gorm.DB
	JWTSecret []byte
}

// NewAuthService creates a new AuthService.
func NewAuthService(db *gorm.DB, secret string) *AuthService {
	return &AuthService{DB: db, JWTSecret: []byte(secret)}
}

// ── JWT ──────────────────────────────────────────────

const tokenTTL = 30 * 24 * time.Hour

// GenerateToken signs a JWT for the given user.
func (s *AuthService) GenerateToken(userID int64, role string) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.JWTSecret)
}

// ValidateToken parses and validates a JWT, then checks revocation against
// the user's updated_at timestamp. Tokens issued before an update are stale.
func (s *AuthService) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{},
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return s.JWTSecret, nil
		})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	// Revocation check. Note: jwt.NumericDate serializes with second precision,
	// while users.updated_at has nanosecond precision — so a token issued in the
	// same second as an account update would compare as iat < updated_at and be
	// wrongly revoked. A 1s tolerance covers that while still invalidating real
	// revocations (password change / force-logout always crosses a second boundary).
	const skewTolerance = time.Second
	var user model.User
	if err := s.DB.Select("updated_at").First(&user, claims.UserID).Error; err != nil {
		return nil, err
	}
	if claims.IssuedAt.Time.Before(user.UpdatedAt.Add(-skewTolerance)) {
		return nil, ErrTokenRevoked
	}
	return claims, nil
}

// ── Password ──────────────────────────────────────────

// HashPassword bcrypt-hashes a plaintext password.
func (s *AuthService) HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether pw matches the stored hash.
func (s *AuthService) CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// ── Users ─────────────────────────────────────────────

// FindUserByID returns a user by primary key.
func (s *AuthService) FindUserByID(id int64) (*model.User, error) {
	var u model.User
	if err := s.DB.First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// FindUserByUsername returns a user by username.
func (s *AuthService) FindUserByUsername(username string) (*model.User, error) {
	var u model.User
	if err := s.DB.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateUser creates a new user with a hashed password.
func (s *AuthService) CreateUser(username, password, role string) (*model.User, error) {
	if role != "admin" && role != "guest" {
		return nil, ErrInvalidRole
	}
	if len(password) < 6 {
		return nil, ErrPasswordTooShort
	}
	hash, err := s.HashPassword(password)
	if err != nil {
		return nil, err
	}
	u := model.User{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
	}
	if err := s.DB.Create(&u).Error; err != nil {
		if isDuplicateKey(err) {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return &u, nil
}

// SeedAdmin creates the initial admin user if none exists.
// No-op when an admin is already present.
func (s *AuthService) SeedAdmin(password string) error {
	var count int64
	s.DB.Model(&model.User{}).Where("role = 'admin'").Count(&count)
	if count > 0 {
		return nil // already seeded
	}
	_, err := s.CreateUser("admin", password, "admin")
	return err
}

// ── Invite Codes ──────────────────────────────────────

const inviteTTL = 7 * 24 * time.Hour

// GenerateInviteCode returns a random hex token for a single-use invite.
func GenerateInviteCode() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// InviteDetail is the API-facing representation of an invite code.
type InviteDetail struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	CreatedBy int64     `json:"created_by"`
	UsedBy    *int64    `json:"used_by"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateInviteCode generates a new single-use invite for a guest.
func (s *AuthService) CreateInviteCode(adminID int64) (*InviteDetail, error) {
	codeStr, err := GenerateInviteCode()
	if err != nil {
		return nil, err
	}
	ic := model.InviteCode{
		Code:      codeStr,
		CreatedBy: adminID,
		ExpiresAt: time.Now().Add(inviteTTL),
	}
	if err := s.DB.Create(&ic).Error; err != nil {
		return nil, err
	}
	return &InviteDetail{
		ID:        ic.ID,
		Code:      ic.Code,
		CreatedBy: ic.CreatedBy,
		UsedBy:    ic.UsedBy,
		ExpiresAt: ic.ExpiresAt,
		CreatedAt: ic.CreatedAt,
	}, nil
}

// ListInviteCodes returns all invites, newest first.
func (s *AuthService) ListInviteCodes() ([]InviteDetail, error) {
	var codes []model.InviteCode
	if err := s.DB.Order("created_at DESC").Find(&codes).Error; err != nil {
		return nil, err
	}
	items := make([]InviteDetail, len(codes))
	for i, c := range codes {
		items[i] = InviteDetail{
			ID:        c.ID,
			Code:      c.Code,
			CreatedBy: c.CreatedBy,
			UsedBy:    c.UsedBy,
			ExpiresAt: c.ExpiresAt,
			CreatedAt: c.CreatedAt,
		}
	}
	return items, nil
}

// DeleteInviteCode removes an invite code (admin revoke).
func (s *AuthService) DeleteInviteCode(id int64) error {
	res := s.DB.Delete(&model.InviteCode{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ValidateInviteCode reports whether an invite code is currently redeemable.
// Returns ErrInviteInvalid when the code is unknown or already used,
// ErrInviteExpired when it has passed its expiry.
func (s *AuthService) ValidateInviteCode(code string) error {
	var ic model.InviteCode
	if err := s.DB.Where("code = ?", code).First(&ic).Error; err != nil {
		return ErrInviteInvalid
	}
	if ic.UsedBy != nil {
		return ErrInviteInvalid
	}
	if time.Now().After(ic.ExpiresAt) {
		return ErrInviteExpired
	}
	return nil
}

// RedeemInviteCode creates a guest user from a valid, unused invite code.
// The code is atomically marked as used in the same transaction.
func (s *AuthService) RedeemInviteCode(code, username, password string) (*model.User, error) {
	var ic model.InviteCode
	if err := s.DB.Where("code = ?", code).First(&ic).Error; err != nil {
		return nil, ErrInviteInvalid
	}
	if ic.UsedBy != nil {
		return nil, ErrInviteInvalid
	}
	if time.Now().After(ic.ExpiresAt) {
		return nil, ErrInviteExpired
	}
	if len(password) < 6 {
		return nil, ErrPasswordTooShort
	}

	tx := s.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	hash, err := s.HashPassword(password)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	u := model.User{
		Username:     username,
		PasswordHash: hash,
		Role:         "guest",
	}
	if err := tx.Create(&u).Error; err != nil {
		tx.Rollback()
		if isDuplicateKey(err) {
			return nil, ErrUserExists
		}
		return nil, err
	}
	ic.UsedBy = &u.ID
	if err := tx.Save(&ic).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// ── User Logs ────────────────────────────────────────

// userLogRetentionMax caps the number of user_logs rows kept in the DB.
const userLogRetentionMax = 2000

// LogEvent records a user activity event (login / session / logout) and
// prunes rows beyond the retention cap.
func (s *AuthService) LogEvent(userID int64, username, eventType, ip string) error {
	if err := s.DB.Create(&model.UserLog{
		UserID:    userID,
		Username:  username,
		EventType: eventType,
		IP:        ip,
	}).Error; err != nil {
		return err
	}

	// Prune efficiently: the id of the userLogRetentionMax-th newest row
	// (PK index scan of only that many rows, unlike a full COUNT(*)). If it
	// exists, there are more rows than the cap — delete everything older.
	// NULL means the table has <= cap rows and nothing to prune.
	var keepFrom *int64
	s.DB.Raw("SELECT id FROM user_logs ORDER BY id DESC LIMIT 1 OFFSET ?", userLogRetentionMax-1).
		Scan(&keepFrom)
	if keepFrom != nil {
		s.DB.Exec("DELETE FROM user_logs WHERE id < ?", *keepFrom)
	}
	return nil
}

// ListLogs returns user activity logs, newest first, with cursor pagination.
func (s *AuthService) ListLogs(limit int, beforeID int64) ([]model.UserLog, int64, error) {
	var total int64
	s.DB.Model(&model.UserLog{}).Count(&total)

	q := s.DB.Order("id DESC").Limit(limit)
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var logs []model.UserLog
	if err := q.Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// isDuplicateKey reports whether err is a Postgres unique-violation (23505).
func isDuplicateKey(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return errors.Is(err, gorm.ErrDuplicatedKey)
}
