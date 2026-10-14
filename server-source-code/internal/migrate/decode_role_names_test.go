package migrate_test

import (
	"context"
	"os"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/google/uuid"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }

func TestDecodeEncodedRoleNames(t *testing.T) {
	d := pgtest.Open(t)
	ctx := context.Background()
	sfx := "-" + uuid.New().String()[:8]
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if _, err := d.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	role := func(name string, manageUsers bool) {
		exec(`INSERT INTO role_permissions (id, role, can_manage_users, updated_at) VALUES ($1, $2, $3, NOW())`, uuid.NewString(), name, manageUsers)
	}
	user := func(roleName string) string {
		id := uuid.NewString()
		exec(`INSERT INTO users (id, username, email, role, updated_at) VALUES ($1, $1, $1 || '@example.com', $2, NOW())`, id, roleName)
		return id
	}

	role("Ops%20Team%20(EU)"+sfx, true)
	renamed := user("Ops%20Team%20(EU)" + sfx)
	role("Dup%20A"+sfx, false)
	role("Dup A"+sfx, false)
	merged := user("Dup%20A" + sfx)
	role("Diff%20B"+sfx, true)
	role("Diff B"+sfx, false)
	kept := user("Diff%20B" + sfx)
	role("Two%20C%28x%29"+sfx, false)
	role("Two%20C(x)"+sfx, false)
	twoA := user("Two%20C%28x%29" + sfx)
	twoB := user("Two%20C(x)" + sfx)
	builtInLookalikes := []string{"%61dmin", "superadmi%6E", "%20User", "Host%5Fmanager"}
	lookalikeUsers := map[string]string{}
	for _, n := range builtInLookalikes {
		role(n, true)
		lookalikeUsers[n] = user(n)
	}
	var adminBefore string
	if err := d.RawQueryRow(ctx, `SELECT (to_jsonb(rp) - 'updated_at')::text FROM role_permissions rp WHERE role = 'admin'`).Scan(&adminBefore); err != nil {
		t.Fatal(err)
	}
	untouched := []string{"bad%ZZ" + sfx, "50%" + sfx, "x%FF" + sfx, "plain_role" + sfx}
	for _, n := range untouched {
		role(n, false)
	}
	session := func(userID string) {
		exec(`INSERT INTO user_sessions (id, user_id, refresh_token, expires_at) VALUES ($1, $2, $1, NOW() + INTERVAL '1 day')`, uuid.NewString(), userID)
	}
	bystander := user("plain_role" + sfx)
	for _, id := range []string{renamed, merged, kept, twoA, twoB, bystander} {
		session(id)
	}
	for _, id := range lookalikeUsers {
		session(id)
	}
	settingsID := uuid.NewString()
	exec(`INSERT INTO settings (id, default_user_role, oidc_default_role, updated_at) VALUES ($1, $2, $2, NOW())`, settingsID, "Ops%20Team%20(EU)"+sfx)

	sql, err := os.ReadFile("migrations/000058_v2-1-4_decode_encoded_role_names.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		exec(string(sql))
	}

	var roles []string
	rows, err := d.Raw(ctx, `SELECT role FROM role_permissions WHERE role LIKE '%' || $1 ORDER BY role`, sfx)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		roles = append(roles, r)
	}
	rows.Close()
	want := map[string]bool{
		"Ops Team (EU)" + sfx: true, "Dup A" + sfx: true, "Diff B" + sfx: true, "Diff%20B" + sfx: true, "Two C(x)" + sfx: true,
	}
	for _, n := range untouched {
		want[n] = true
	}
	if len(roles) != len(want) {
		t.Fatalf("roles = %q, want %d rows", roles, len(want))
	}
	for _, r := range roles {
		if !want[r] {
			t.Errorf("unexpected role %q", r)
		}
	}

	userRole := func(id string) (name string, manageUsers bool) {
		t.Helper()
		if err := d.RawQueryRow(ctx, `SELECT u.role, rp.can_manage_users FROM users u JOIN role_permissions rp ON rp.role = u.role WHERE u.id = $1`, id).Scan(&name, &manageUsers); err != nil {
			t.Fatalf("user %s no longer resolves to a role: %v", id, err)
		}
		return name, manageUsers
	}
	for id, w := range map[string]struct {
		name   string
		manage bool
	}{
		renamed: {"Ops Team (EU)" + sfx, true},
		merged:  {"Dup A" + sfx, false},
		kept:    {"Diff%20B" + sfx, true},
		twoA:    {"Two C(x)" + sfx, false},
		twoB:    {"Two C(x)" + sfx, false},
	} {
		if name, manage := userRole(id); name != w.name || manage != w.manage {
			t.Errorf("user role = %q (manage users %v), want %q (%v)", name, manage, w.name, w.manage)
		}
	}

	sessions := func(id string) int {
		t.Helper()
		var n int
		if err := d.RawQueryRow(ctx, `SELECT COUNT(*) FROM user_sessions WHERE user_id = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, id := range []string{renamed, merged, twoA, twoB} {
		if n := sessions(id); n != 0 {
			t.Errorf("user %s moved to a new role name still has %d session(s)", id, n)
		}
	}
	for _, id := range []string{kept, bystander} {
		if n := sessions(id); n != 1 {
			t.Errorf("user %s was not moved but has %d session(s), want 1", id, n)
		}
	}

	for n, id := range lookalikeUsers {
		var exists bool
		if err := d.RawQueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM role_permissions WHERE role = $1)`, n).Scan(&exists); err != nil || !exists {
			t.Errorf("role %q decoding to a built-in role was renamed or merged (err %v)", n, err)
		}
		if name, _ := userRole(id); name != n {
			t.Errorf("user on %q moved to %q", n, name)
		}
		if c := sessions(id); c != 1 {
			t.Errorf("user on %q lost their session: %d", n, c)
		}
	}
	var adminAfter string
	if err := d.RawQueryRow(ctx, `SELECT (to_jsonb(rp) - 'updated_at')::text FROM role_permissions rp WHERE role = 'admin'`).Scan(&adminAfter); err != nil {
		t.Fatal(err)
	}
	if adminAfter != adminBefore {
		t.Errorf("built-in admin role changed:\n%s\n%s", adminBefore, adminAfter)
	}

	var defRole, oidcRole string
	if err := d.RawQueryRow(ctx, `SELECT default_user_role, oidc_default_role FROM settings WHERE id = $1`, settingsID).Scan(&defRole, &oidcRole); err != nil {
		t.Fatal(err)
	}
	if defRole != "Ops Team (EU)"+sfx || oidcRole != defRole {
		t.Errorf("settings roles = %q, %q", defRole, oidcRole)
	}

	var fn int
	if err := d.RawQueryRow(ctx, `SELECT COUNT(*) FROM pg_proc WHERE proname = 'pm_tmp_pct_decode'`).Scan(&fn); err != nil || fn != 0 {
		t.Errorf("temporary decode function left behind: count=%d err=%v", fn, err)
	}
}
