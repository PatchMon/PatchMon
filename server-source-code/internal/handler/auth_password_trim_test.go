package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func storedPasswordIs(t *testing.T, users *store.UsersStore, userID, want string) bool {
	t.Helper()
	u, err := users.GetByID(context.Background(), userID)
	if err != nil || u == nil || u.PasswordHash == nil {
		t.Fatalf("load user %s: %v", userID, err)
	}
	return bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(want)) == nil
}

func TestChangePasswordTrimsLikeLogin(t *testing.T) {
	f := newLoginFixture(t, 1000)
	ctx := context.Background()
	u, err := f.h.users.GetByUsernameOrEmail(ctx, f.active)
	if err != nil {
		t.Fatal(err)
	}
	change := func(cur, next string) int {
		t.Helper()
		body := `{"currentPassword":"` + cur + `","newPassword":"` + next + `"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/change-password", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, u.ID))
		rec := httptest.NewRecorder()
		f.h.ChangePassword(rec, req)
		return rec.Code
	}
	const spaced = "  pw with Spaces 9!  "
	if got := change(" "+hardeningPassword+" ", spaced); got != http.StatusOK {
		t.Fatalf("change with a padded current password: got %d", got)
	}
	for _, pw := range []string{spaced, strings.TrimSpace(spaced)} {
		if rec := postLogin(f.h, f.active, pw); rec.Code != http.StatusOK {
			t.Fatalf("login with %q after setting %q: got %d %s", pw, spaced, rec.Code, rec.Body.String())
		}
	}
	if got := change(strings.TrimSpace(spaced), " "+strings.TrimSpace(spaced)+"   "); got != http.StatusBadRequest {
		t.Fatalf("padding the current password is not a new password: got %d", got)
	}
	if got := change(strings.TrimSpace(spaced), "      Ab1!x      "); got != http.StatusBadRequest {
		t.Fatalf("policy must apply to the trimmed password: got %d", got)
	}
}

func TestAdminSetPasswordsAreTrimmed(t *testing.T) {
	d := testDatabase(t)
	users := store.NewUsersStore(d)
	uh := &UsersHandler{users: users, sessions: store.NewSessionsStore(d), permissions: store.NewPermissionsStore(d)}
	call := func(h http.HandlerFunc, path, userID, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("userId", userID)
		c := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		c = context.WithValue(c, middleware.UserRoleKey, "superadmin")
		c = context.WithValue(c, middleware.UserIDKey, "someone-else")
		rec := httptest.NewRecorder()
		h(rec, req.WithContext(c))
		return rec
	}

	rec := call(uh.Create, "/auth/admin/users", "", `{"username":"trim_create","email":"trim_create@ex.test","password":"  Created-Passw0rd!  ","role":"user"}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	created, err := users.GetByUsernameOrEmail(context.Background(), "trim_create")
	if err != nil || created == nil {
		t.Fatalf("load created user: %v", err)
	}
	if !storedPasswordIs(t, users, created.ID, "Created-Passw0rd!") {
		t.Error("user create stored the untrimmed password")
	}

	rec = call(uh.ResetPassword, "/auth/admin/users/"+created.ID+"/reset-password", created.ID, `{"newPassword":"  Reset-Passw0rd!  "}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", rec.Code, rec.Body.String())
	}
	if !storedPasswordIs(t, users, created.ID, "Reset-Passw0rd!") {
		t.Error("admin password reset stored the untrimmed password")
	}

	rec = call(uh.ResetPassword, "/auth/admin/users/"+created.ID+"/reset-password", created.ID, `{"newPassword":"     Ab1!x     "}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("reset policy must apply to the trimmed password: got %d", rec.Code)
	}
}

func TestTfaResetTemporaryPasswordIsTrimmed(t *testing.T) {
	ctx := context.Background()
	d := testDatabase(t)
	x := newTfaResetHarness(t, d, d)
	admin := x.mkUser(ctx, "admin", false)
	target := x.mkUser(ctx, "user", true)
	rec := x.post(ctx, admin, target.ID, map[string]string{"password": " " + tfaResetTestPassword + " ", "newPassword": "  Temp-Passw0rd!x  "})
	if rec.Code != http.StatusOK {
		t.Fatalf("reset with padded passwords: %d %s", rec.Code, rec.Body.String())
	}
	if !storedPasswordIs(t, x.users, target.ID, "Temp-Passw0rd!x") {
		t.Error("temporary password stored untrimmed")
	}
}
