//go:build integration

package service

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"shutterseek/internal/model"
)

func setupAuthSvc(t *testing.T) *AuthService {
	t.Helper()
	dsn := testDSN()
	if dsn == "" {
		t.Skip("database env vars not set (SHUTTERSEEK_DB_USER etc.)")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	// Clean up leftover test users and invite codes
	db.Exec("DELETE FROM invite_codes WHERE created_by IN (SELECT id FROM users WHERE username LIKE 'TEST_%')")
	db.Exec("DELETE FROM users WHERE username LIKE 'TEST_%'")
	return NewAuthService(db, "test-secret-key")
}

// deleteUser removes a user, first clearing any invite_codes that reference it
// (used_by FK) to avoid foreign-key violations during cleanup.
func deleteUser(db *gorm.DB, id int64) {
	db.Exec("DELETE FROM invite_codes WHERE used_by = ?", id)
	db.Where("id = ?", id).Delete(&model.User{})
}

// testDSN builds a Postgres DSN from SHUTTERSEEK_* env vars.
// Empty when credentials are unavailable (tests will skip).
func testDSN() string {
	user := os.Getenv("SHUTTERSEEK_DB_USER")
	pass := os.Getenv("SHUTTERSEEK_DB_PASSWORD")
	name := os.Getenv("SHUTTERSEEK_DB_NAME")
	host := os.Getenv("SHUTTERSEEK_DB_HOST")
	if host == "" {
		host = "postgres-main"
	}
	if user == "" || pass == "" || name == "" {
		return ""
	}
	return fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable", user, pass, host, name)
}

// ═══════════════════════════════════════════════════════
// Password
// ═══════════════════════════════════════════════════════

func TestHashAndCheckPassword(t *testing.T) {
	svc := setupAuthSvc(t)
	hash, err := svc.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "secret123" {
		t.Fatal("hash must not equal plaintext")
	}
	if !svc.CheckPassword(hash, "secret123") {
		t.Fatal("correct password should pass")
	}
	if svc.CheckPassword(hash, "wrong") {
		t.Fatal("wrong password should fail")
	}
}

// ═══════════════════════════════════════════════════════
// JWT
// ═══════════════════════════════════════════════════════

func TestGenerateAndValidateToken(t *testing.T) {
	svc := setupAuthSvc(t)
	u, err := svc.CreateUser("TEST_jwt_user", "secret123", "guest")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer deleteUser(svc.DB, u.ID)

	token, err := svc.GenerateToken(u.ID, u.Role)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if claims.UserID != u.ID || claims.Role != "guest" {
		t.Fatalf("wrong claims: %+v", claims)
	}
}

func TestValidateToken_Tampered(t *testing.T) {
	svc := setupAuthSvc(t)
	u, err := svc.CreateUser("TEST_jwt_tamper", "secret123", "guest")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer deleteUser(svc.DB, u.ID)

	token, _ := svc.GenerateToken(u.ID, u.Role)
	// Corrupt the payload (change a char in the middle)
	bad := token[:len(token)-4] + "xxxx"
	if _, err := svc.ValidateToken(bad); err == nil {
		t.Fatal("tampered token should fail")
	}
}

func TestValidateToken_Revoked(t *testing.T) {
	svc := setupAuthSvc(t)
	u, err := svc.CreateUser("TEST_jwt_revoke", "secret123", "guest")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer deleteUser(svc.DB, u.ID)

	token, _ := svc.GenerateToken(u.ID, u.Role)

	// Touch updated_at to revoke all prior tokens
	svc.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("updated_at", time.Now().Add(1*time.Hour))

	if _, err := svc.ValidateToken(token); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("expected ErrTokenRevoked, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════
// Users
// ═══════════════════════════════════════════════════════

func TestCreateUser_Success(t *testing.T) {
	svc := setupAuthSvc(t)
	u, err := svc.CreateUser("TEST_create_ok", "secret123", "guest")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer deleteUser(svc.DB, u.ID)
	if u.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if u.Role != "guest" {
		t.Fatalf("wrong role: %s", u.Role)
	}
	if u.PasswordHash == "secret123" {
		t.Fatal("password must be hashed")
	}
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	svc := setupAuthSvc(t)
	u, err := svc.CreateUser("TEST_create_dup", "secret123", "guest")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	defer deleteUser(svc.DB, u.ID)

	if _, err := svc.CreateUser("TEST_create_dup", "other123", "guest"); !errors.Is(err, ErrUserExists) {
		t.Fatalf("expected ErrUserExists, got %v", err)
	}
}

func TestCreateUser_ShortPassword(t *testing.T) {
	svc := setupAuthSvc(t)
	if _, err := svc.CreateUser("TEST_create_short", "abc", "guest"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestCreateUser_InvalidRole(t *testing.T) {
	svc := setupAuthSvc(t)
	if _, err := svc.CreateUser("TEST_create_role", "secret123", "superuser"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("expected ErrInvalidRole, got %v", err)
	}
}

func TestFindUserByUsername(t *testing.T) {
	svc := setupAuthSvc(t)
	u, err := svc.CreateUser("TEST_find_by_name", "secret123", "guest")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer deleteUser(svc.DB, u.ID)

	found, err := svc.FindUserByUsername("TEST_find_by_name")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found.ID != u.ID {
		t.Fatalf("wrong user: %d vs %d", found.ID, u.ID)
	}
}

// ═══════════════════════════════════════════════════════
// SeedAdmin
// ═══════════════════════════════════════════════════════

func TestSeedAdmin_NoAdmin(t *testing.T) {
	svc := setupAuthSvc(t)
	if err := svc.SeedAdmin("initial-admin-pw"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var count int64
	svc.DB.Model(&model.User{}).Where("role = 'admin'").Count(&count)
	if count == 0 {
		t.Fatal("expected at least one admin")
	}
	// login check
	u, _ := svc.FindUserByUsername("admin")
	if u != nil {
		deleteUser(svc.DB, u.ID)
	}
}

// ═══════════════════════════════════════════════════════
// Invite Codes
// ═══════════════════════════════════════════════════════

func TestCreateAndListInviteCodes(t *testing.T) {
	svc := setupAuthSvc(t)
	admin, err := svc.CreateUser("TEST_inv_admin", "secret123", "admin")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	defer deleteUser(svc.DB, admin.ID)

	detail, err := svc.CreateInviteCode(admin.ID)
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if len(detail.Code) != 32 {
		t.Fatalf("expected 32-char code, got %d", len(detail.Code))
	}
	if detail.UsedBy != nil {
		t.Fatal("new invite should be unused")
	}

	items, err := svc.ListInviteCodes()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, it := range items {
		if it.ID == detail.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("invite code not in list")
	}
	svc.DB.Where("id = ?", detail.ID).Delete(&model.InviteCode{})
}

func TestDeleteInviteCode(t *testing.T) {
	svc := setupAuthSvc(t)
	admin, _ := svc.CreateUser("TEST_inv_del_admin", "secret123", "admin")
	defer deleteUser(svc.DB, admin.ID)

	detail, _ := svc.CreateInviteCode(admin.ID)
	if err := svc.DeleteInviteCode(detail.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.DeleteInviteCode(detail.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected not found on second delete, got %v", err)
	}
}

func TestRedeemInviteCode_Success(t *testing.T) {
	svc := setupAuthSvc(t)
	admin, _ := svc.CreateUser("TEST_rd_admin", "secret123", "admin")
	defer deleteUser(svc.DB, admin.ID)
	detail, _ := svc.CreateInviteCode(admin.ID)
	defer svc.DB.Where("id = ?", detail.ID).Delete(&model.InviteCode{})

	u, err := svc.RedeemInviteCode(detail.Code, "TEST_rd_guest", "guestpass1")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	defer deleteUser(svc.DB, u.ID)
	if u.Role != "guest" {
		t.Fatalf("expected guest role, got %s", u.Role)
	}

	// Code should now be marked used
	var ic model.InviteCode
	svc.DB.First(&ic, detail.ID)
	if ic.UsedBy == nil || *ic.UsedBy != u.ID {
		t.Fatal("invite code should be marked used")
	}
}

func TestRedeemInviteCode_AlreadyUsed(t *testing.T) {
	svc := setupAuthSvc(t)
	admin, _ := svc.CreateUser("TEST_rd2_admin", "secret123", "admin")
	defer deleteUser(svc.DB, admin.ID)
	detail, _ := svc.CreateInviteCode(admin.ID)
	defer svc.DB.Where("id = ?", detail.ID).Delete(&model.InviteCode{})

	u1, _ := svc.RedeemInviteCode(detail.Code, "TEST_rd2_g1", "guestpass1")
	defer deleteUser(svc.DB, u1.ID)

	if _, err := svc.RedeemInviteCode(detail.Code, "TEST_rd2_g2", "guestpass2"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid for used code, got %v", err)
	}
}

func TestRedeemInviteCode_Expired(t *testing.T) {
	svc := setupAuthSvc(t)
	admin, _ := svc.CreateUser("TEST_rd3_admin", "secret123", "admin")
	defer deleteUser(svc.DB, admin.ID)

	// Create an already-expired invite directly
	ic := model.InviteCode{
		Code:      "expired-code-1234567890",
		CreatedBy: admin.ID,
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	svc.DB.Create(&ic)
	defer svc.DB.Where("id = ?", ic.ID).Delete(&model.InviteCode{})

	if _, err := svc.RedeemInviteCode(ic.Code, "TEST_rd3_g", "guestpass1"); !errors.Is(err, ErrInviteExpired) {
		t.Fatalf("expected ErrInviteExpired, got %v", err)
	}
}

func TestRedeemInviteCode_UnknownCode(t *testing.T) {
	svc := setupAuthSvc(t)
	if _, err := svc.RedeemInviteCode("nonexistent-code", "TEST_rd4_g", "guestpass1"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════
// Invite Validation
// ═══════════════════════════════════════════════════════

func TestValidateInviteCode_Valid(t *testing.T) {
	svc := setupAuthSvc(t)
	admin, _ := svc.CreateUser("TEST_vl_admin", "secret123", "admin")
	defer deleteUser(svc.DB, admin.ID)
	detail, _ := svc.CreateInviteCode(admin.ID)
	defer svc.DB.Where("id = ?", detail.ID).Delete(&model.InviteCode{})

	if err := svc.ValidateInviteCode(detail.Code); err != nil {
		t.Fatalf("valid code should pass, got %v", err)
	}
}

func TestValidateInviteCode_Used(t *testing.T) {
	svc := setupAuthSvc(t)
	admin, _ := svc.CreateUser("TEST_vl2_admin", "secret123", "admin")
	defer deleteUser(svc.DB, admin.ID)
	detail, _ := svc.CreateInviteCode(admin.ID)
	defer svc.DB.Where("id = ?", detail.ID).Delete(&model.InviteCode{})

	u, _ := svc.RedeemInviteCode(detail.Code, "TEST_vl2_g", "guestpass1")
	defer deleteUser(svc.DB, u.ID)

	if err := svc.ValidateInviteCode(detail.Code); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("used code should be invalid, got %v", err)
	}
}

func TestValidateInviteCode_Expired(t *testing.T) {
	svc := setupAuthSvc(t)
	admin, _ := svc.CreateUser("TEST_vl3_admin", "secret123", "admin")
	defer deleteUser(svc.DB, admin.ID)
	ic := model.InviteCode{
		Code:      "expired-validate-code-123",
		CreatedBy: admin.ID,
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	svc.DB.Create(&ic)
	defer svc.DB.Where("id = ?", ic.ID).Delete(&model.InviteCode{})

	if err := svc.ValidateInviteCode(ic.Code); !errors.Is(err, ErrInviteExpired) {
		t.Fatalf("expected ErrInviteExpired, got %v", err)
	}
}

func TestValidateInviteCode_Unknown(t *testing.T) {
	svc := setupAuthSvc(t)
	if err := svc.ValidateInviteCode("nonexistent-validate-code"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid, got %v", err)
	}
}
