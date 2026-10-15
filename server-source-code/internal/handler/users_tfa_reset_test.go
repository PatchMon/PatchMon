package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/migrate"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const tfaResetTestPassword = "Actor-Passw0rd!"

type tfaResetHarness struct {
	t        *testing.T
	d        *database.DB
	users    *store.UsersStore
	sessions *store.SessionsStore
	trusted  *store.TrustedDevicesStore
	perms    *store.PermissionsStore
	lockout  *store.TfaLockoutStore
	replay   *store.TfaReplayStore
	h        *UsersHandler
}

type tfaTestUser struct {
	*models.User
	secret      string
	backupCodes []string
}

func newTfaResetHarness(t *testing.T, provider database.DBProvider, d *database.DB) *tfaResetHarness {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	lockout := store.NewTfaLockoutStore(&hostctx.RedisResolver{Default: rc}, nil, 3, 30)

	users := store.NewUsersStore(provider)
	sessions := store.NewSessionsStore(provider)
	trusted := store.NewTrustedDevicesStore(provider)
	perms := store.NewPermissionsStore(provider)
	h := NewUsersHandler(users, sessions, trusted, perms, nil, nil, nil, provider, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithTfaReset(lockout, store.NewAuditLogsStore(provider), store.NewTfaReplayStore(&hostctx.RedisResolver{Default: rc}))
	return &tfaResetHarness{t: t, d: d, users: users, sessions: sessions, trusted: trusted, perms: perms, lockout: lockout, replay: h.tfaReplay, h: h}
}

func (x *tfaResetHarness) mkUser(ctx context.Context, role string, withTfa bool) *tfaTestUser {
	x.t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(tfaResetTestPassword), bcrypt.MinCost)
	if err != nil {
		x.t.Fatal(err)
	}
	hs := string(hash)
	u := &models.User{
		ID:           uuid.New().String(),
		Username:     "u" + uuid.New().String()[:12],
		Email:        uuid.New().String()[:12] + "@example.com",
		PasswordHash: &hs,
		Role:         role,
		IsActive:     true,
	}
	if err := x.users.Create(ctx, u); err != nil {
		x.t.Fatal(err)
	}
	out := &tfaTestUser{User: u}
	if withTfa {
		secret, _, err := util.GenerateTOTPSecret("PatchMon", u.Username)
		if err != nil {
			x.t.Fatal(err)
		}
		if err := x.users.UpdateTfaSecret(ctx, u.ID, &secret); err != nil {
			x.t.Fatal(err)
		}
		codes := util.GenerateBackupCodes(util.BackupCodeCount)
		hashed := make([]string, len(codes))
		for i, c := range codes {
			h, err := bcrypt.GenerateFromPassword([]byte(c), bcrypt.MinCost)
			if err != nil {
				x.t.Fatal(err)
			}
			hashed[i] = string(h)
		}
		js := util.EncodeBackupCodesJSON(hashed)
		if err := x.users.UpdateTfaEnabled(ctx, u.ID, &js); err != nil {
			x.t.Fatal(err)
		}
		out.secret, out.backupCodes = secret, codes
	}
	return out
}

func (x *tfaResetHarness) mkSession(ctx context.Context, userID string) string {
	x.t.Helper()
	s, err := x.sessions.Create(ctx, userID, uuid.New().String(), uuid.New().String(), "198.51.100.1", "test", "", "", time.Now().Add(time.Hour), false, nil)
	if err != nil {
		x.t.Fatal(err)
	}
	return s.ID
}

func (x *tfaResetHarness) mkTrustedDevice(ctx context.Context, userID string) {
	x.t.Helper()
	if _, err := x.trusted.Create(ctx, store.CreateTrustedDeviceParams{UserID: userID, TokenHash: uuid.New().String(), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		x.t.Fatal(err)
	}
}

func (x *tfaResetHarness) post(ctx context.Context, actor *tfaTestUser, targetID string, body any) *httptest.ResponseRecorder {
	x.t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/admin/users/"+targetID+"/reset-tfa", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("userId", targetID)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, middleware.UserIDKey, actor.ID)
	ctx = context.WithValue(ctx, middleware.UserRoleKey, actor.Role)
	ctx = context.WithValue(ctx, middleware.RequestIDKey, "req-"+uuid.New().String()[:8])
	rec := httptest.NewRecorder()
	x.h.ResetTfa(rec, req.WithContext(ctx))
	return rec
}

func (x *tfaResetHarness) reload(ctx context.Context, id string) *models.User {
	x.t.Helper()
	u, err := x.users.GetByID(ctx, id)
	if err != nil {
		x.t.Fatal(err)
	}
	return u
}

func (x *tfaResetHarness) count(ctx context.Context, sql string, args ...any) int {
	x.t.Helper()
	var n int
	if err := x.d.RawQueryRow(ctx, sql, args...).Scan(&n); err != nil {
		x.t.Fatal(err)
	}
	return n
}

func (x *tfaResetHarness) assertTfaIntact(ctx context.Context, target *tfaTestUser) {
	x.t.Helper()
	u := x.reload(ctx, target.ID)
	if !u.TfaEnabled || u.TfaSecret == nil || *u.TfaSecret != target.secret {
		x.t.Fatalf("target TFA was modified: enabled=%v", u.TfaEnabled)
	}
}

func currentCode(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func errorOf(rec *httptest.ResponseRecorder) string {
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	s, _ := m["error"].(string)
	return s
}

func TestAdminResetTfa(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	x := newTfaResetHarness(t, d, d)
	pw := map[string]string{"password": tfaResetTestPassword}

	t.Run("admin resets a user and everything is cleared", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", true)
		adminSession := x.mkSession(ctx, admin.ID)
		x.mkSession(ctx, target.ID)
		x.mkSession(ctx, target.ID)
		x.mkTrustedDevice(ctx, target.ID)
		x.lockout.RecordFailedAttempt(ctx, target.ID)

		rec := x.post(ctx, admin, target.ID, pw)
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
		}
		u := x.reload(ctx, target.ID)
		if u.TfaEnabled || u.TfaSecret != nil || u.TfaBackupCodes != nil {
			t.Fatalf("TFA not cleared: enabled=%v secret=%v codes=%v", u.TfaEnabled, u.TfaSecret, u.TfaBackupCodes)
		}
		if !u.TfaEnrolmentRequired || !u.TfaEnrolmentPending() {
			t.Fatal("local account was not flagged for re-enrolment")
		}
		if !strings.Contains(rec.Body.String(), `"enrolmentRequired":true`) {
			t.Fatalf("response does not report enrolment: %s", rec.Body.String())
		}
		js := `[]`
		if err := x.users.UpdateTfaEnabled(ctx, target.ID, &js); err != nil {
			t.Fatal(err)
		}
		if x.reload(ctx, target.ID).TfaEnrolmentRequired {
			t.Fatal("completing enrolment did not clear the flag")
		}
		if n := x.count(ctx, `SELECT count(*) FROM user_sessions WHERE user_id=$1 AND is_revoked=false`, target.ID); n != 0 {
			t.Fatalf("%d target sessions still live", n)
		}
		if n := x.count(ctx, `SELECT count(*) FROM user_trusted_devices WHERE user_id=$1 AND is_revoked=false`, target.ID); n != 0 {
			t.Fatalf("%d target trusted devices still live", n)
		}
		if n := x.count(ctx, `SELECT count(*) FROM user_sessions WHERE id=$1 AND is_revoked=false`, adminSession); n != 1 {
			t.Fatal("actor's own session was revoked")
		}
		if n := x.count(ctx, `SELECT count(*) FROM audit_logs WHERE event='user_tfa_reset' AND user_id=$1 AND target_user_id=$2 AND success AND request_id LIKE 'req-%'`, admin.ID, target.ID); n != 1 {
			t.Fatalf("want one success audit row, got %d", n)
		}
		if attempts, _ := x.lockout.RecordFailedAttempt(ctx, target.ID); attempts != 1 {
			t.Fatalf("target failed-attempt counter not cleared, now %d", attempts)
		}
	})

	t.Run("reset lifts an active TFA lockout on the target", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", true)
		for range 3 {
			x.lockout.RecordFailedAttempt(ctx, target.ID)
		}
		if locked, _ := x.lockout.IsTFALocked(ctx, target.ID); !locked {
			t.Fatal("setup: target should be locked")
		}
		if rec := x.post(ctx, admin, target.ID, pw); rec.Code != http.StatusOK {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		if locked, _ := x.lockout.IsTFALocked(ctx, target.ID); locked {
			t.Fatal("target is still locked out after the reset")
		}
	})

	t.Run("actor with TFA must supply a valid TOTP", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", true)
		target := x.mkUser(ctx, "user", true)

		rec := x.post(ctx, admin, target.ID, pw)
		if rec.Code != http.StatusForbidden || !strings.Contains(errorOf(rec), "code is required") {
			t.Fatalf("missing code: got %d %s", rec.Code, rec.Body.String())
		}
		rec = x.post(ctx, admin, target.ID, map[string]string{"password": tfaResetTestPassword, "tfaToken": "000000"})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("wrong code: got %d %s", rec.Code, rec.Body.String())
		}
		x.assertTfaIntact(ctx, target)

		rec = x.post(ctx, admin, target.ID, map[string]string{"password": tfaResetTestPassword, "tfaToken": currentCode(t, admin.secret)})
		if rec.Code != http.StatusOK {
			t.Fatalf("valid code: got %d %s", rec.Code, rec.Body.String())
		}
		if x.reload(ctx, target.ID).TfaEnabled {
			t.Fatal("target TFA still enabled")
		}
		if !x.reload(ctx, admin.ID).TfaEnabled {
			t.Fatal("actor's own TFA was touched")
		}
	})

	t.Run("actor backup code is accepted once", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", true)
		t1 := x.mkUser(ctx, "user", true)
		t2 := x.mkUser(ctx, "user", true)
		body := map[string]string{"password": tfaResetTestPassword, "tfaToken": strings.ToLower(admin.backupCodes[0])}

		if rec := x.post(ctx, admin, t1.ID, body); rec.Code != http.StatusOK {
			t.Fatalf("backup code: got %d %s", rec.Code, rec.Body.String())
		}
		if rec := x.post(ctx, admin, t2.ID, body); rec.Code != http.StatusForbidden {
			t.Fatalf("reused backup code: got %d %s", rec.Code, rec.Body.String())
		}
		x.assertTfaIntact(ctx, t2)
	})

	t.Run("a temporary password is set in the same reset and must be changed", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", true)
		rec := x.post(ctx, admin, target.ID, map[string]string{"password": tfaResetTestPassword, "newPassword": "Temp-Passw0rd!x"})
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"passwordChangeRequired":true`) {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		u := x.reload(ctx, target.ID)
		if bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(tfaResetTestPassword)) == nil {
			t.Fatal("old password still valid")
		}
		if bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte("Temp-Passw0rd!x")) != nil {
			t.Fatal("temporary password not set")
		}
		if !u.PasswordChangePending() || !u.TfaEnrolmentPending() || u.TfaEnabled {
			t.Fatalf("flags: password=%v enrol=%v tfa=%v", u.PasswordChangePending(), u.TfaEnrolmentPending(), u.TfaEnabled)
		}
		if n := x.count(ctx, `SELECT count(*) FROM audit_logs WHERE target_user_id=$1 AND success AND details LIKE '%"temporary_password":true%'`, target.ID); n != 1 {
			t.Fatalf("want an audit row recording the temporary password, got %d", n)
		}
		if err := x.users.UpdatePassword(ctx, target.ID, *u.PasswordHash); err != nil {
			t.Fatal(err)
		}
		if x.reload(ctx, target.ID).PasswordChangeRequired {
			t.Fatal("changing the password did not clear the flag")
		}
	})

	t.Run("without a temporary password the password is untouched", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", true)
		before := *x.reload(ctx, target.ID).PasswordHash
		if rec := x.post(ctx, admin, target.ID, pw); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"passwordChangeRequired":false`) {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		u := x.reload(ctx, target.ID)
		if *u.PasswordHash != before || u.PasswordChangeRequired {
			t.Fatal("password changed or flagged without the option")
		}
	})

	t.Run("a temporary password must meet the policy and needs a local account", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", true)
		if rec := x.post(ctx, admin, target.ID, map[string]string{"password": tfaResetTestPassword, "newPassword": "x"}); rec.Code != http.StatusBadRequest {
			t.Fatalf("weak password: got %d %s", rec.Code, rec.Body.String())
		}
		if n := x.count(ctx, `SELECT count(*) FROM audit_logs WHERE target_user_id=$1 AND NOT success AND details = '{"reason":"temp_password_policy"}'`, target.ID); n != 1 {
			t.Fatalf("want one temp_password_policy audit row with no password in it, got %d", n)
		}
		x.assertTfaIntact(ctx, target)
		sso := x.mkUser(ctx, "user", true)
		if _, err := d.Exec(ctx, `UPDATE users SET password_hash=NULL, oidc_sub=$1 WHERE id=$2`, "sub-"+sso.ID, sso.ID); err != nil {
			t.Fatal(err)
		}
		if rec := x.post(ctx, admin, sso.ID, map[string]string{"password": tfaResetTestPassword, "newPassword": "Temp-Passw0rd!x"}); rec.Code != http.StatusBadRequest {
			t.Fatalf("SSO-only target: got %d %s", rec.Code, rec.Body.String())
		}
		if n := x.count(ctx, `SELECT count(*) FROM audit_logs WHERE target_user_id=$1 AND NOT success AND details LIKE '%temp_password_sso_target%' AND details NOT LIKE '%Temp-Passw0rd%'`, sso.ID); n != 1 {
			t.Fatalf("want one temp_password_sso_target audit row, got %d", n)
		}
		x.assertTfaIntact(ctx, sso)
	})

	t.Run("an actor's TOTP code works once", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", true)
		t1 := x.mkUser(ctx, "user", true)
		t2 := x.mkUser(ctx, "user", true)
		body := map[string]string{"password": tfaResetTestPassword, "tfaToken": currentCode(t, admin.secret)}
		if rec := x.post(ctx, admin, t1.ID, body); rec.Code != http.StatusOK {
			t.Fatalf("first use: got %d %s", rec.Code, rec.Body.String())
		}
		if rec := x.post(ctx, admin, t2.ID, body); rec.Code != http.StatusForbidden {
			t.Fatalf("replayed code: got %d %s", rec.Code, rec.Body.String())
		}
		x.assertTfaIntact(ctx, t2)
	})

	t.Run("repeated wrong codes lock the actor out", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", true)
		target := x.mkUser(ctx, "user", true)
		bad := map[string]string{"password": tfaResetTestPassword, "tfaToken": "ZZZZZZ"}
		codes := []int{}
		for range 4 {
			codes = append(codes, x.post(ctx, admin, target.ID, bad).Code)
		}
		want := []int{403, 403, 429, 429}
		for i := range want {
			if codes[i] != want[i] {
				t.Fatalf("attempt codes = %v, want %v", codes, want)
			}
		}
		rec := x.post(ctx, admin, target.ID, map[string]string{"password": tfaResetTestPassword, "tfaToken": currentCode(t, admin.secret)})
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("valid code while locked: got %d", rec.Code)
		}
		x.assertTfaIntact(ctx, target)
		if n := x.count(ctx, `SELECT count(*) FROM audit_logs WHERE user_id=$1 AND NOT success AND details LIKE '%tfa_locked_out%'`, admin.ID); n != 3 {
			t.Fatalf("want 3 locked-out audit rows, got %d", n)
		}
	})

	t.Run("wrong password is rejected and audited", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", true)
		rec := x.post(ctx, admin, target.ID, map[string]string{"password": "nope"})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		x.assertTfaIntact(ctx, target)
		if n := x.count(ctx, `SELECT count(*) FROM audit_logs WHERE user_id=$1 AND target_user_id=$2 AND NOT success AND details LIKE '%invalid_password%'`, admin.ID, target.ID); n != 1 {
			t.Fatalf("want one failed audit row, got %d", n)
		}
	})

	t.Run("empty password and malformed body", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", true)
		if rec := x.post(ctx, admin, target.ID, map[string]string{}); rec.Code != http.StatusBadRequest {
			t.Fatalf("empty password: got %d", rec.Code)
		}
		if rec := x.post(ctx, admin, target.ID, "not an object"); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad body: got %d", rec.Code)
		}
		x.assertTfaIntact(ctx, target)
	})

	t.Run("own account is refused", func(t *testing.T) {
		admin := x.mkUser(ctx, "superadmin", true)
		rec := x.post(ctx, admin, admin.ID, map[string]string{"password": tfaResetTestPassword, "tfaToken": currentCode(t, admin.secret)})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		x.assertTfaIntact(ctx, admin)
	})

	t.Run("role hierarchy", func(t *testing.T) {
		readonly, err := x.perms.GetByRole(ctx, "readonly")
		if err != nil {
			t.Fatal(err)
		}
		auditor, other := *readonly, *readonly
		auditor.Role, auditor.CanViewUsers = "custom_auditor", true
		other.Role, other.CanManageAlerts = "custom_other", true
		for _, p := range []*models.RolePermission{&auditor, &other} {
			if err := x.perms.UpsertRole(ctx, p); err != nil {
				t.Fatal(err)
			}
		}
		cases := []struct {
			actor, target string
			want          int
		}{
			{"superadmin", "superadmin", http.StatusOK},
			{"superadmin", "admin", http.StatusOK},
			{"admin", "host_manager", http.StatusOK},
			{"admin", "user", http.StatusOK},
			{"admin", "custom_auditor", http.StatusOK},
			{"host_manager", "user", http.StatusOK},
			{"custom_auditor", "readonly", http.StatusOK},
			{"admin", "admin", http.StatusForbidden},
			{"admin", "superadmin", http.StatusForbidden},
			{"host_manager", "host_manager", http.StatusForbidden},
			{"host_manager", "admin", http.StatusForbidden},
			{"custom_auditor", "custom_other", http.StatusForbidden},
			{"custom_auditor", "user", http.StatusForbidden},
			{"user", "user", http.StatusForbidden},
			{"user", "readonly", http.StatusOK},
		}
		for _, tc := range cases {
			t.Run(tc.actor+"->"+tc.target, func(t *testing.T) {
				actor := x.mkUser(ctx, tc.actor, false)
				target := x.mkUser(ctx, tc.target, true)
				rec := x.post(ctx, actor, target.ID, pw)
				if rec.Code != tc.want {
					t.Fatalf("got %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
				}
				if tc.want != http.StatusOK {
					x.assertTfaIntact(ctx, target)
				}
			})
		}
	})

	t.Run("role is read from the database, not the token", func(t *testing.T) {
		actor := x.mkUser(ctx, "user", false)
		target := x.mkUser(ctx, "host_manager", true)
		actor.Role = "superadmin"
		if rec := x.post(ctx, actor, target.ID, pw); rec.Code != http.StatusForbidden {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		x.assertTfaIntact(ctx, target)
	})

	t.Run("unknown user is 404 only after re-authentication", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		missing := uuid.New().String()
		if rec := x.post(ctx, admin, missing, map[string]string{"password": "nope"}); rec.Code != http.StatusForbidden {
			t.Fatalf("wrong password, unknown target: got %d", rec.Code)
		}
		if rec := x.post(ctx, admin, missing, pw); rec.Code != http.StatusNotFound {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		if n := x.count(ctx, `SELECT count(*) FROM audit_logs WHERE user_id=$1 AND target_user_id=$2 AND NOT success AND details LIKE '%target_not_found%'`, admin.ID, missing); n != 1 {
			t.Fatalf("want one target_not_found audit row, got %d", n)
		}
	})

	t.Run("target without TFA", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", false)
		session := x.mkSession(ctx, target.ID)
		if rec := x.post(ctx, admin, target.ID, pw); rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		if n := x.count(ctx, `SELECT count(*) FROM audit_logs WHERE user_id=$1 AND target_user_id=$2 AND NOT success AND details LIKE '%target_tfa_not_enabled%'`, admin.ID, target.ID); n != 1 {
			t.Fatalf("want one target_tfa_not_enabled audit row, got %d", n)
		}
		if n := x.count(ctx, `SELECT count(*) FROM user_sessions WHERE id=$1 AND is_revoked=false`, session); n != 1 {
			t.Fatal("sessions revoked although nothing was reset")
		}
	})

	t.Run("inactive target can be reset", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", true)
		target.IsActive = false
		if err := x.users.Update(ctx, target.User); err != nil {
			t.Fatal(err)
		}
		if rec := x.post(ctx, admin, target.ID, pw); rec.Code != http.StatusOK {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("OIDC-linked target with local TFA can be reset", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", true)
		if _, err := d.Exec(ctx, `UPDATE users SET oidc_sub=$1, oidc_provider='idp.example.com' WHERE id=$2`, "sub-"+target.ID, target.ID); err != nil {
			t.Fatal(err)
		}
		if rec := x.post(ctx, admin, target.ID, pw); rec.Code != http.StatusOK {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		if x.reload(ctx, target.ID).TfaEnrolmentRequired {
			t.Fatal("IdP-linked account must not be forced to enrol local 2FA")
		}
	})

	t.Run("legacy provider-only OIDC target is not forced to enrol", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", true)
		if _, err := d.Exec(ctx, `UPDATE users SET oidc_provider='idp.example.com' WHERE id=$1`, target.ID); err != nil {
			t.Fatal(err)
		}
		if rec := x.post(ctx, admin, target.ID, pw); rec.Code != http.StatusOK {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		if x.reload(ctx, target.ID).TfaEnrolmentRequired {
			t.Fatal("a provider-only IdP link cannot set up local 2FA, so it must not be required")
		}
	})

	t.Run("OIDC-only actor cannot re-authenticate", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", true)
		if _, err := d.Exec(ctx, `UPDATE users SET password_hash=NULL, oidc_sub=$1 WHERE id=$2`, "sub-"+admin.ID, admin.ID); err != nil {
			t.Fatal(err)
		}
		if rec := x.post(ctx, admin, target.ID, pw); rec.Code != http.StatusForbidden {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		x.assertTfaIntact(ctx, target)
		if n := x.count(ctx, `SELECT count(*) FROM audit_logs WHERE user_id=$1 AND NOT success AND details LIKE '%actor_no_password%'`, admin.ID); n != 1 {
			t.Fatalf("want one actor_no_password audit row, got %d", n)
		}
	})

	t.Run("deactivated actor is refused", func(t *testing.T) {
		admin := x.mkUser(ctx, "admin", false)
		target := x.mkUser(ctx, "user", true)
		admin.IsActive = false
		if err := x.users.Update(ctx, admin.User); err != nil {
			t.Fatal(err)
		}
		if rec := x.post(ctx, admin, target.ID, pw); rec.Code != http.StatusUnauthorized {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		x.assertTfaIntact(ctx, target)
		if n := x.count(ctx, `SELECT count(*) FROM audit_logs WHERE user_id=$1 AND NOT success AND details LIKE '%actor_inactive%'`, admin.ID); n != 1 {
			t.Fatalf("want one actor_inactive audit row, got %d", n)
		}
	})
}

func TestAdminResetTfaRequiresManageUsers(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	x := newTfaResetHarness(t, d, d)
	target := x.mkUser(ctx, "readonly", true)

	for _, tc := range []struct {
		role string
		want int
	}{
		{"user", http.StatusForbidden},
		{"readonly", http.StatusForbidden},
		{"", http.StatusUnauthorized},
	} {
		t.Run("role="+tc.role, func(t *testing.T) {
			r := chi.NewRouter()
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					c := context.WithValue(req.Context(), middleware.UserIDKey, uuid.New().String())
					c = context.WithValue(c, middleware.UserRoleKey, tc.role)
					next.ServeHTTP(w, req.WithContext(c))
				})
			})
			r.With(middleware.RequirePermission("can_manage_users", x.perms)).Post("/auth/admin/users/{userId}/reset-tfa", x.h.ResetTfa)
			req := httptest.NewRequest(http.MethodPost, "/auth/admin/users/"+target.ID+"/reset-tfa", strings.NewReader(`{"password":"`+tfaResetTestPassword+`"}`))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
	x.assertTfaIntact(ctx, target)
}

type tfaCtxKey struct{}

type twoContextProvider struct{ a, b *database.DB }

func (p twoContextProvider) DB(ctx context.Context) *database.DB {
	if ctx.Value(tfaCtxKey{}) == "b" {
		return p.b
	}
	return p.a
}

func TestAdminResetTfaCannotReachAnotherContext(t *testing.T) {
	ctx := context.Background()
	a := pgtest.Open(t)
	b := openSecondTestDB(t)
	x := newTfaResetHarness(t, twoContextProvider{a: a, b: b}, a)

	admin := x.mkUser(ctx, "superadmin", false)
	ctxB := context.WithValue(ctx, tfaCtxKey{}, "b")
	other := x.mkUser(ctxB, "user", true)

	rec := x.post(ctx, admin, other.ID, map[string]string{"password": tfaResetTestPassword})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	x.assertTfaIntact(ctxB, other)
}

func openSecondTestDB(t *testing.T) *database.DB {
	t.Helper()
	u, err := url.Parse(pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	name := strings.TrimPrefix(u.Path, "/") + "_ctxb"
	admin := *u
	admin.Path = "/postgres"
	exec := func(sql string) {
		conn, err := pgx.Connect(context.Background(), admin.String())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close(context.Background()) }()
		if _, err := conn.Exec(context.Background(), sql); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE DATABASE " + pgx.Identifier{name}.Sanitize())
	u.Path = "/" + name
	if err := migrate.Run(u.String(), slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	d, err := database.NewFromURL(context.Background(), u.String(), 2, 1, &config.Config{DBConnectTimeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		d.Close()
		exec("DROP DATABASE IF EXISTS " + pgx.Identifier{name}.Sanitize() + " WITH (FORCE)")
	})
	return d
}

func TestAdminUsersListHidesTfaStateWithoutManageUsers(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	x := newTfaResetHarness(t, d, d)
	if _, err := d.Exec(ctx, `INSERT INTO role_permissions (id, role, can_view_users, can_manage_users, created_at, updated_at)
		VALUES (gen_random_uuid()::text, 'list_viewer', true, false, now(), now()) ON CONFLICT (role) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	x.mkUser(ctx, "user", true)
	list := func(role string) []map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/admin/users?all=true", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserRoleKey, role))
		rec := httptest.NewRecorder()
		x.h.List(rec, req)
		var body struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Data) == 0 {
			t.Fatalf("list as %s: %d %s", role, rec.Code, rec.Body.String())
		}
		return body.Data
	}
	for _, u := range list("list_viewer") {
		if _, ok := u["tfa_enabled"]; ok {
			t.Fatal("tfa_enabled returned to a caller without can_manage_users")
		}
	}
	for _, u := range list("admin") {
		if _, ok := u["tfa_enabled"]; !ok {
			t.Fatal("tfa_enabled missing for a caller with can_manage_users")
		}
	}
}

func TestResetPasswordDuringTfaResetWindow(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	x := newTfaResetHarness(t, d, d)
	resetPassword := func(actor *tfaTestUser, targetID string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/admin/users/"+targetID+"/reset-password", strings.NewReader(`{"newPassword":"Attacker-Passw0rd!"}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("userId", targetID)
		c := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		c = context.WithValue(c, middleware.UserIDKey, actor.ID)
		c = context.WithValue(c, middleware.UserRoleKey, actor.Role)
		rec := httptest.NewRecorder()
		x.h.ResetPassword(rec, req.WithContext(c))
		return rec.Code
	}
	super := x.mkUser(ctx, "superadmin", false)
	adminy := x.mkUser(ctx, "admin", false)

	for _, temp := range []bool{false, true} {
		adminx := x.mkUser(ctx, "admin", true)
		body := map[string]string{"password": tfaResetTestPassword}
		if temp {
			body["newPassword"] = "Temp-Passw0rd!x"
		}
		if rec := x.post(ctx, super, adminx.ID, body); rec.Code != http.StatusOK {
			t.Fatalf("2FA reset: %d %s", rec.Code, rec.Body.String())
		}
		if got := resetPassword(adminy, adminx.ID); got != http.StatusForbidden {
			t.Fatalf("temp=%v: equal-rank admin reset the password mid-window: got %d", temp, got)
		}
		forged := *adminy
		forged.Role = "superadmin"
		if got := resetPassword(&forged, adminx.ID); got != http.StatusForbidden {
			t.Fatalf("temp=%v: token role trusted over the database: got %d", temp, got)
		}
		if got := resetPassword(super, adminx.ID); got != http.StatusOK {
			t.Fatalf("temp=%v: superadmin refused: got %d", temp, got)
		}
	}

	t.Run("superadmin may act on another superadmin mid-reset", func(t *testing.T) {
		other := x.mkUser(ctx, "superadmin", true)
		if rec := x.post(ctx, super, other.ID, map[string]string{"password": tfaResetTestPassword}); rec.Code != http.StatusOK {
			t.Fatalf("2FA reset: %d %s", rec.Code, rec.Body.String())
		}
		if got := resetPassword(super, other.ID); got != http.StatusOK {
			t.Fatalf("superadmin on superadmin: got %d", got)
		}
	})

	t.Run("ordinary targets are unchanged", func(t *testing.T) {
		peer := x.mkUser(ctx, "admin", true)
		if got := resetPassword(adminy, peer.ID); got != http.StatusOK {
			t.Fatalf("equal-rank reset of an ordinary account: got %d", got)
		}
	})
}
