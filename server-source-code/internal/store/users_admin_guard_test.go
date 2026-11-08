package store

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
)

func seedUser(t *testing.T, s *UsersStore, role string, active bool) *models.User {
	t.Helper()
	u := &models.User{
		ID:       uuid.New().String(),
		Username: "u" + uuid.New().String()[:12],
		Email:    "u" + uuid.New().String()[:8] + "@example.com",
		Role:     role,
		IsActive: active,
	}
	if err := s.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestCreateFirstAdminLatchSurvivesUserRemoval(t *testing.T) {
	ctx := context.Background()
	d := pgtest.OpenEmpty(t)
	s := NewUsersStore(d)

	first := &models.User{Username: "first", Email: "first@example.com", Role: "superadmin", IsActive: true}
	if err := s.CreateFirstAdmin(ctx, first, AuditEntry{Event: "first_user_created", Success: true}); err != nil {
		t.Fatal(err)
	}
	var audits int
	if err := d.RawQueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE event = 'first_user_created' AND target_user_id = $1`, first.ID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("audit rows = %d, want 1", audits)
	}
	if _, err := d.Exec(ctx, `DELETE FROM users`); err != nil {
		t.Fatal(err)
	}
	done, err := s.SetupCompleted(ctx)
	if err != nil || !done {
		t.Fatalf("SetupCompleted = %v, %v; want true with no users left", done, err)
	}
	again := &models.User{Username: "again", Email: "again@example.com", Role: "superadmin", IsActive: true}
	if err := s.CreateFirstAdmin(ctx, again, AuditEntry{Event: "first_user_created"}); !errors.Is(err, ErrSetupCompleted) {
		t.Fatalf("second CreateFirstAdmin err = %v, want ErrSetupCompleted", err)
	}
}

func TestCreateFirstAdminRefusesWhenUsersExistWithoutLatch(t *testing.T) {
	ctx := context.Background()
	d := pgtest.OpenEmpty(t)
	s := NewUsersStore(d)
	seedUser(t, s, "user", false)
	u := &models.User{Username: "late", Email: "late@example.com", Role: "superadmin", IsActive: true}
	if err := s.CreateFirstAdmin(ctx, u, AuditEntry{Event: "first_user_created"}); !errors.Is(err, ErrSetupCompleted) {
		t.Fatalf("err = %v, want ErrSetupCompleted", err)
	}
}

func TestInstallStateMigrationBackfillsInstallsWithUsers(t *testing.T) {
	ctx := context.Background()
	d := pgtest.OpenEmpty(t)
	s := NewUsersStore(d)
	up, err := os.ReadFile("../migrate/migrations/000064_v2-1-9_install_state.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	latched := func() bool {
		t.Helper()
		var n int
		if err := d.RawQueryRow(ctx, `SELECT COUNT(*) FROM install_state`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if _, err := d.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	if latched() {
		t.Fatal("empty install was latched by the migration")
	}
	seedUser(t, s, "superadmin", false)
	if _, err := d.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	if !latched() {
		t.Fatal("install with only an inactive superadmin was not latched")
	}
}

func TestUpdateKeepingAdminRefusesLastActiveAdministrator(t *testing.T) {
	ctx := context.Background()
	d := pgtest.OpenEmpty(t)
	s := NewUsersStore(d)
	super := seedUser(t, s, "superadmin", true)
	admin := seedUser(t, s, "admin", true)

	demoted := *super
	demoted.Role = "admin"
	if err := s.UpdateKeepingAdmin(ctx, &demoted); !errors.Is(err, ErrLastSuperadmin) {
		t.Fatalf("demote last superadmin: err = %v, want ErrLastSuperadmin", err)
	}
	off := *super
	off.IsActive = false
	if err := s.UpdateKeepingAdmin(ctx, &off); !errors.Is(err, ErrLastSuperadmin) {
		t.Fatalf("deactivate last superadmin: err = %v, want ErrLastSuperadmin", err)
	}
	got, err := s.GetByID(ctx, super.ID)
	if err != nil || got.Role != "superadmin" || !got.IsActive {
		t.Fatalf("refused update was applied: %+v, %v", got, err)
	}

	adminOff := *admin
	adminOff.IsActive = false
	if err := s.UpdateKeepingAdmin(ctx, &adminOff); err != nil {
		t.Fatalf("deactivate admin while a superadmin remains: %v", err)
	}
}

func TestUpdateKeepingAdminRefusesLastAdminWithoutSuperadmin(t *testing.T) {
	ctx := context.Background()
	d := pgtest.OpenEmpty(t)
	s := NewUsersStore(d)
	admin := seedUser(t, s, "admin", true)
	demoted := *admin
	demoted.Role = "user"
	if err := s.UpdateKeepingAdmin(ctx, &demoted); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}
}

func TestUpdateKeepingAdminAllowsUpdatesThatRemoveNoAdministrator(t *testing.T) {
	ctx := context.Background()
	d := pgtest.OpenEmpty(t)
	s := NewUsersStore(d)
	member := seedUser(t, s, "host_manager", true)
	other := seedUser(t, s, "user", true)
	dormant := seedUser(t, s, "superadmin", false)

	renamed := *member
	renamed.Username = "renamed" + uuid.New().String()[:8]
	if err := s.UpdateKeepingAdmin(ctx, &renamed); err != nil {
		t.Fatalf("rename with no administrators: %v", err)
	}
	off := *other
	off.IsActive = false
	if err := s.UpdateKeepingAdmin(ctx, &off); err != nil {
		t.Fatalf("deactivate ordinary user with no administrators: %v", err)
	}
	dormantRenamed := *dormant
	dormantRenamed.Username = "dormant" + uuid.New().String()[:8]
	if err := s.UpdateKeepingAdmin(ctx, &dormantRenamed); err != nil {
		t.Fatalf("rename inactive superadmin: %v", err)
	}
	dormantDemoted := dormantRenamed
	dormantDemoted.Role = "user"
	if err := s.UpdateKeepingAdmin(ctx, &dormantDemoted); err != nil {
		t.Fatalf("demote inactive superadmin: %v", err)
	}
}

func TestUpdateKeepingAdminMissingUser(t *testing.T) {
	d := pgtest.OpenEmpty(t)
	s := NewUsersStore(d)
	ghost := &models.User{ID: uuid.New().String(), Username: "ghost", Email: "ghost@example.com", Role: "user", IsActive: true}
	if err := s.UpdateKeepingAdmin(context.Background(), ghost); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("err = %v, want pgx.ErrNoRows", err)
	}
}

func TestDeleteKeepingAdminGuardsOnlyActiveAdministrators(t *testing.T) {
	ctx := context.Background()
	d := pgtest.OpenEmpty(t)
	s := NewUsersStore(d)
	super := seedUser(t, s, "superadmin", true)
	if err := s.DeleteKeepingAdmin(ctx, super.ID); !errors.Is(err, ErrLastSuperadmin) {
		t.Fatalf("delete last active superadmin: err = %v, want ErrLastSuperadmin", err)
	}
	inactiveSuper := seedUser(t, s, "superadmin", false)
	if err := s.DeleteKeepingAdmin(ctx, inactiveSuper.ID); err != nil {
		t.Fatalf("delete inactive superadmin: %v", err)
	}
	if _, err := d.Exec(ctx, `DELETE FROM users WHERE id = $1`, super.ID); err != nil {
		t.Fatal(err)
	}

	admin := seedUser(t, s, "admin", true)
	if err := s.DeleteKeepingAdmin(ctx, admin.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("delete last active admin: err = %v, want ErrLastAdmin", err)
	}
	inactiveAdmin := seedUser(t, s, "admin", false)
	if err := s.DeleteKeepingAdmin(ctx, inactiveAdmin.ID); err != nil {
		t.Fatalf("delete inactive admin: %v", err)
	}
}

func TestConcurrentDeactivationKeepsOneSuperadmin(t *testing.T) {
	ctx := context.Background()
	d := pgtest.OpenEmpty(t)
	s := NewUsersStore(d)
	a := seedUser(t, s, "superadmin", true)
	b := seedUser(t, s, "superadmin", true)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, u := range []*models.User{a, b} {
		wg.Add(1)
		go func(i int, u models.User) {
			defer wg.Done()
			u.IsActive = false
			errs[i] = s.UpdateKeepingAdmin(ctx, &u)
		}(i, *u)
	}
	wg.Wait()
	assertOneSucceeded(t, errs, ErrLastSuperadmin)
	if n, _ := s.CountSuperadmins(ctx); n != 1 {
		t.Fatalf("active superadmins = %d, want 1", n)
	}
}

func TestConcurrentDeleteKeepsOneSuperadmin(t *testing.T) {
	ctx := context.Background()
	d := pgtest.OpenEmpty(t)
	s := NewUsersStore(d)
	a := seedUser(t, s, "superadmin", true)
	b := seedUser(t, s, "superadmin", true)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{a.ID, b.ID} {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			errs[i] = s.DeleteKeepingAdmin(ctx, id)
		}(i, id)
	}
	wg.Wait()
	assertOneSucceeded(t, errs, ErrLastSuperadmin)
	if n, _ := s.CountSuperadmins(ctx); n != 1 {
		t.Fatalf("active superadmins = %d, want 1", n)
	}
}

func assertOneSucceeded(t *testing.T, errs []error, want error) {
	t.Helper()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, want):
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("successes = %d, want 1 (%v)", ok, errs)
	}
}
