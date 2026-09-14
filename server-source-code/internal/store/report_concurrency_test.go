package store

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Both BulkUpsertPackages and UpsertRepository skip their DO UPDATE when the
// reported values are unchanged, which means RETURNING can come back empty for
// a row that does exist. The id then has to be resolved by a SEPARATE
// statement: a fallback inside the same statement reads at the same snapshot
// as the upsert and cannot see rows a concurrent report committed after that
// snapshot was taken.
//
// UpsertRepository additionally must not run at all on an unchanged report:
// ON CONFLICT locks the conflicting tuple before it evaluates the DO UPDATE
// WHERE, so a no-op upsert still pins the fleet-shared repository row for the
// life of the report transaction. These tests pin both behaviours and the SQL
// shape they depend on.

// repoRow is a fake pgx.Row for GetRepositoryByURLDistComponents. A nil row
// means the lookup should report no rows.
type repoRow struct {
	id       string
	name     string
	repoType string
	isActive bool
	isSecure bool
	priority *int32
	desc     *string
	err      error
}

func (r repoRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 7 {
		return errors.New("repoRow: unexpected destination count")
	}
	*(dest[0].(*string)) = r.id
	*(dest[1].(*string)) = r.name
	*(dest[2].(*string)) = r.repoType
	*(dest[3].(*bool)) = r.isActive
	*(dest[4].(*bool)) = r.isSecure
	*(dest[5].(**int32)) = r.priority
	*(dest[6].(**string)) = r.desc
	return nil
}

// idRow is a fake pgx.Row for UpsertRepository's RETURNING id.
type idRow struct {
	id  string
	err error
}

func (r idRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("idRow: unexpected destination count")
	}
	*(dest[0].(*string)) = r.id
	return nil
}

// fakeDBTX routes QueryRow by statement text so the read-then-upsert path can
// be exercised without a live database. lookupRows is consumed in order, so a
// test can give different answers to the pre-check and the follow-up resolve.
type fakeDBTX struct {
	lookupRows []repoRow
	upsertRow  idRow
	upserts    int
	lookups    int
}

func (f *fakeDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("fakeDBTX: Exec not supported")
}

func (f *fakeDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, errors.New("fakeDBTX: Query not supported")
}

func (f *fakeDBTX) QueryRow(_ context.Context, sql string, _ ...interface{}) pgx.Row {
	if strings.Contains(sql, "INSERT INTO repositories") {
		f.upserts++
		return f.upsertRow
	}
	i := f.lookups
	f.lookups++
	if i >= len(f.lookupRows) {
		return repoRow{err: errors.New("fakeDBTX: unexpected extra lookup")}
	}
	return f.lookupRows[i]
}

func testUpsertRepositoryParams() db.UpsertRepositoryParams {
	desc := "deb repository for noble"
	return db.UpsertRepositoryParams{
		ID:           "new-id",
		Name:         "archive",
		Url:          "http://archive.ubuntu.com/ubuntu",
		Distribution: "noble",
		Components:   "main restricted",
		RepoType:     "deb",
		IsActive:     true,
		IsSecure:     true,
		Description:  &desc,
	}
}

// matchingRepoRow is what the pre-check reads back for a steady-state report.
func matchingRepoRow(id string) repoRow {
	p := testUpsertRepositoryParams()
	return repoRow{
		id:       id,
		name:     p.Name,
		repoType: p.RepoType,
		isActive: p.IsActive,
		isSecure: p.IsSecure,
		priority: p.Priority,
		desc:     p.Description,
	}
}

// The whole point of the fix: an unchanged report must not touch the upsert,
// because ON CONFLICT would lock the fleet-shared row regardless of its WHERE.
func TestUpsertRepositoryResolvingID_UnchangedRowSkipsUpsertEntirely(t *testing.T) {
	t.Parallel()

	f := &fakeDBTX{
		lookupRows: []repoRow{matchingRepoRow("existing-repo-id")},
		upsertRow:  idRow{err: errors.New("upsert must not run for an unchanged repository")},
	}
	got, err := upsertRepositoryResolvingID(context.Background(), db.New(f), testUpsertRepositoryParams())
	if err != nil {
		t.Fatalf("upsertRepositoryResolvingID: %v", err)
	}
	if got != "existing-repo-id" {
		t.Fatalf("got repository id %q, want %q", got, "existing-repo-id")
	}
	if f.upserts != 0 {
		t.Fatalf("upsert ran %d times for an unchanged repository, want 0 (it would take the hot-row lock)", f.upserts)
	}
}

func TestUpsertRepositoryResolvingID_MissingRowRunsUpsert(t *testing.T) {
	t.Parallel()

	f := &fakeDBTX{
		lookupRows: []repoRow{{err: pgx.ErrNoRows}},
		upsertRow:  idRow{id: "inserted-repo-id"},
	}
	got, err := upsertRepositoryResolvingID(context.Background(), db.New(f), testUpsertRepositoryParams())
	if err != nil {
		t.Fatalf("upsertRepositoryResolvingID: %v", err)
	}
	if got != "inserted-repo-id" {
		t.Fatalf("got repository id %q, want %q", got, "inserted-repo-id")
	}
	if f.upserts != 1 {
		t.Fatalf("upsert ran %d times for a missing repository, want 1", f.upserts)
	}
}

func TestUpsertRepositoryResolvingID_ChangedColumnRunsUpsert(t *testing.T) {
	t.Parallel()

	stale := matchingRepoRow("existing-repo-id")
	stale.name = "archive-old"

	f := &fakeDBTX{
		lookupRows: []repoRow{stale},
		upsertRow:  idRow{id: "existing-repo-id"},
	}
	got, err := upsertRepositoryResolvingID(context.Background(), db.New(f), testUpsertRepositoryParams())
	if err != nil {
		t.Fatalf("upsertRepositoryResolvingID: %v", err)
	}
	if got != "existing-repo-id" {
		t.Fatalf("got repository id %q, want %q", got, "existing-repo-id")
	}
	if f.upserts != 1 {
		t.Fatalf("upsert ran %d times for a changed repository, want 1", f.upserts)
	}
}

// A concurrent report can commit the same change between the pre-check and the
// upsert; the upsert's skip-no-op WHERE then suppresses RETURNING and the id
// has to come from a fresh-snapshot statement.
func TestUpsertRepositoryResolvingID_SuppressedReturningResolvesInSecondStatement(t *testing.T) {
	t.Parallel()

	stale := matchingRepoRow("existing-repo-id")
	stale.name = "archive-old"

	f := &fakeDBTX{
		lookupRows: []repoRow{stale, matchingRepoRow("existing-repo-id")},
		upsertRow:  idRow{err: pgx.ErrNoRows},
	}
	got, err := upsertRepositoryResolvingID(context.Background(), db.New(f), testUpsertRepositoryParams())
	if err != nil {
		t.Fatalf("upsertRepositoryResolvingID: %v", err)
	}
	if got != "existing-repo-id" {
		t.Fatalf("got repository id %q, want %q", got, "existing-repo-id")
	}
	if f.lookups != 2 {
		t.Fatalf("expected a pre-check and one follow-up resolve, got %d lookups", f.lookups)
	}
}

func TestUpsertRepositoryResolvingID_StillMissingAfterResolveErrors(t *testing.T) {
	t.Parallel()

	f := &fakeDBTX{
		lookupRows: []repoRow{{err: pgx.ErrNoRows}, {err: pgx.ErrNoRows}},
		upsertRow:  idRow{err: pgx.ErrNoRows},
	}
	_, err := upsertRepositoryResolvingID(context.Background(), db.New(f), testUpsertRepositoryParams())
	if err == nil {
		t.Fatal("expected an error when the repository is missing after the follow-up resolve")
	}
	if !strings.Contains(err.Error(), "resolve unchanged repository") {
		t.Fatalf("error should identify the resolve step, got: %v", err)
	}
}

func TestUpsertRepositoryResolvingID_PropagatesUpsertErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("deadlock detected")
	f := &fakeDBTX{
		lookupRows: []repoRow{{err: pgx.ErrNoRows}},
		upsertRow:  idRow{err: boom},
	}
	_, err := upsertRepositoryResolvingID(context.Background(), db.New(f), testUpsertRepositoryParams())
	if !errors.Is(err, boom) {
		t.Fatalf("upsert error should propagate unwrapped by the no-rows path, got: %v", err)
	}
	if f.lookups != 1 {
		t.Fatalf("follow-up resolve ran after a real upsert error (%d lookups, want 1)", f.lookups)
	}
}

func TestUpsertRepositoryResolvingID_PropagatesPreCheckErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("connection reset")
	f := &fakeDBTX{
		lookupRows: []repoRow{{err: boom}},
		upsertRow:  idRow{err: errors.New("upsert must not run after a failed pre-check")},
	}
	_, err := upsertRepositoryResolvingID(context.Background(), db.New(f), testUpsertRepositoryParams())
	if !errors.Is(err, boom) {
		t.Fatalf("pre-check error should propagate, got: %v", err)
	}
	if f.upserts != 0 {
		t.Fatalf("upsert ran %d times after a failed pre-check, want 0", f.upserts)
	}
}

func TestRepositoryRowMatches(t *testing.T) {
	t.Parallel()

	str := func(s string) *string { return &s }
	i32 := func(i int32) *int32 { return &i }

	tests := []struct {
		name   string
		mutate func(*repoRow, *db.UpsertRepositoryParams)
		want   bool
	}{
		{name: "identical", mutate: func(*repoRow, *db.UpsertRepositoryParams) {}, want: true},
		{name: "name differs", mutate: func(r *repoRow, _ *db.UpsertRepositoryParams) { r.name = "other" }},
		{name: "repo_type differs", mutate: func(r *repoRow, _ *db.UpsertRepositoryParams) { r.repoType = "rpm" }},
		{name: "is_active differs", mutate: func(r *repoRow, _ *db.UpsertRepositoryParams) { r.isActive = false }},
		{name: "is_secure differs", mutate: func(r *repoRow, _ *db.UpsertRepositoryParams) { r.isSecure = false }},
		{name: "stored priority against reported nil", mutate: func(r *repoRow, _ *db.UpsertRepositoryParams) { r.priority = i32(5) }},
		{name: "reported priority against stored nil", mutate: func(_ *repoRow, p *db.UpsertRepositoryParams) { p.Priority = i32(5) }},
		{name: "priority values differ", mutate: func(r *repoRow, p *db.UpsertRepositoryParams) {
			r.priority, p.Priority = i32(1), i32(2)
		}},
		{name: "equal priorities", mutate: func(r *repoRow, p *db.UpsertRepositoryParams) {
			r.priority, p.Priority = i32(7), i32(7)
		}, want: true},
		{name: "description differs", mutate: func(r *repoRow, _ *db.UpsertRepositoryParams) { r.desc = str("stale") }},
		// COALESCE(EXCLUDED.description, repositories.description): a nil
		// reported description never overwrites, so it is never a difference.
		{name: "nil reported description keeps stored", mutate: func(r *repoRow, p *db.UpsertRepositoryParams) {
			r.desc, p.Description = str("stored"), nil
		}, want: true},
		{name: "reported description against stored nil", mutate: func(r *repoRow, _ *db.UpsertRepositoryParams) { r.desc = nil }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			row, params := matchingRepoRow("id"), testUpsertRepositoryParams()
			tc.mutate(&row, &params)
			got := repositoryRowMatches(db.GetRepositoryByURLDistComponentsRow{
				ID:          row.id,
				Name:        row.name,
				RepoType:    row.repoType,
				IsActive:    row.isActive,
				IsSecure:    row.isSecure,
				Priority:    row.priority,
				Description: row.desc,
			}, params)
			if got != tc.want {
				t.Fatalf("repositoryRowMatches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMissingPackageNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		packages []ReportPackage
		nameToID map[string]string
		want     []string
	}{
		{
			name:     "all resolved returns nil",
			packages: []ReportPackage{{Name: "bind9"}, {Name: "cryptsetup"}},
			nameToID: map[string]string{"bind9": "p1", "cryptsetup": "p2"},
			want:     nil,
		},
		{
			name:     "reports only the unresolved names",
			packages: []ReportPackage{{Name: "bind9"}, {Name: "cryptsetup"}, {Name: "zsh"}},
			nameToID: map[string]string{"bind9": "p1"},
			want:     []string{"cryptsetup", "zsh"},
		},
		{
			name:     "empty map returns every name",
			packages: []ReportPackage{{Name: "bind9"}, {Name: "cryptsetup"}},
			nameToID: map[string]string{},
			want:     []string{"bind9", "cryptsetup"},
		},
		{
			name:     "duplicate names are collapsed",
			packages: []ReportPackage{{Name: "zsh"}, {Name: "zsh"}, {Name: "zsh"}},
			nameToID: map[string]string{},
			want:     []string{"zsh"},
		},
		{
			name:     "no packages returns nil",
			packages: nil,
			nameToID: map[string]string{},
			want:     nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := missingPackageNames(tc.packages, tc.nameToID)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("missingPackageNames = %v, want %v", got, tc.want)
			}
		})
	}
}

// repositoryUpdatedColumns are the columns UpsertRepository's DO UPDATE
// writes. They must stay in step with repositoryRowMatches, which decides
// whether the caller can skip the statement altogether.
var repositoryUpdatedColumns = []string{"name", "repo_type", "is_active", "is_secure", "priority", "description"}

func TestUpsertRepository_SkipsNoOpUpdates(t *testing.T) {
	t.Parallel()

	stmt := extractStatement(t, readQueryFile(t, "../sqlc/queries/repositories.sql"), "UpsertRepository")

	doUpdate := strings.Index(stmt, "DO UPDATE SET")
	if doUpdate < 0 {
		t.Fatal("UpsertRepository must remain an ON CONFLICT ... DO UPDATE upsert")
	}
	if !strings.Contains(stmt[doUpdate:], "WHERE") {
		t.Fatal("UpsertRepository's DO UPDATE must keep its skip-no-op WHERE")
	}

	// Every column the DO UPDATE sets must appear in the predicate, otherwise a
	// changed value can be silently skipped.
	for _, col := range repositoryUpdatedColumns {
		re := regexp.MustCompile(`repositories\.` + col + `\s+IS DISTINCT FROM`)
		if !re.MatchString(stmt) {
			t.Errorf("column %q is written by DO UPDATE but is not compared in the skip-no-op WHERE", col)
		}
	}

	// And the reverse. Without this, adding a column to the SET clause without
	// adding it here leaves it out of both the predicate and
	// repositoryRowMatches: the pre-check then reports "unchanged" for a real
	// change and the write is dropped with no error anywhere. It is the only
	// way this design fails silently, so pin it.
	setBlock := stmt[doUpdate+len("DO UPDATE SET"):]
	if w := strings.Index(setBlock, "WHERE"); w >= 0 {
		setBlock = setBlock[:w]
	}
	var written []string
	for _, m := range regexp.MustCompile(`(?m)^\s*([a-z_]+)\s*=`).FindAllStringSubmatch(setBlock, -1) {
		// updated_at is bookkeeping: always NOW(), never a change signal.
		if m[1] == "updated_at" {
			continue
		}
		written = append(written, m[1])
	}
	declared := slices.Clone(repositoryUpdatedColumns)
	slices.Sort(written)
	slices.Sort(declared)
	if !slices.Equal(written, declared) {
		t.Errorf("DO UPDATE writes %v but repositoryUpdatedColumns declares %v; "+
			"an undeclared column is ignored by repositoryRowMatches and its changes are silently dropped",
			written, declared)
	}

	// The no-rows case must be resolved by a separate statement, never by a
	// fallback stitched into this one (that is the BulkUpsertPackages snapshot
	// trap).
	if strings.Contains(stmt, "UNION ALL") {
		t.Error("UpsertRepository must not resolve skipped rows with an in-statement fallback SELECT; " +
			"it reads at the same snapshot as the upsert and cannot see concurrently committed rows")
	}
}

// The pre-check that keeps unchanged reports off the hot row can only be
// trusted if it reads every column the upsert would write.
func TestGetRepositoryByURLDistComponents_ProjectsEveryUpdatedColumn(t *testing.T) {
	t.Parallel()

	stmt := extractStatement(t, readQueryFile(t, "../sqlc/queries/repositories.sql"), "GetRepositoryByURLDistComponents")

	if strings.Contains(stmt, "FOR UPDATE") || strings.Contains(stmt, "FOR NO KEY UPDATE") || strings.Contains(stmt, "FOR SHARE") {
		t.Fatal("the pre-check must stay a lock-free read; taking a row lock here reinstates the serialisation it exists to avoid")
	}
	for _, col := range repositoryUpdatedColumns {
		if !regexp.MustCompile(`\b` + col + `\b`).MatchString(stmt) {
			t.Errorf("column %q is written by UpsertRepository but not read by the pre-check, so a change to it would be missed", col)
		}
	}
}

// BulkUpsertPackages keeps its skip-no-op WHERE (roughly 95% of steady-state
// upsert calls become no-ops); the Go caller compensates for the resulting
// gaps in RETURNING instead.
func TestBulkUpsertPackages_KeepsSkipNoOpAndHasSeparateResolver(t *testing.T) {
	t.Parallel()

	sql := readQueryFile(t, "../sqlc/queries/host_report.sql")
	stmt := extractStatement(t, sql, "BulkUpsertPackages")

	doUpdate := strings.Index(stmt, "DO UPDATE SET")
	if doUpdate < 0 {
		t.Fatal("BulkUpsertPackages must remain an ON CONFLICT ... DO UPDATE upsert")
	}
	if !strings.Contains(stmt[doUpdate:], "IS DISTINCT FROM") {
		t.Fatal("BulkUpsertPackages must keep its skip-no-op WHERE; removing it restores heavy WAL " +
			"and lock-conflict pressure on the shared packages table")
	}

	resolver := extractStatement(t, sql, "GetPackageIDsByNames")
	if !strings.Contains(resolver, "FROM packages") || !strings.Contains(resolver, "ANY(") {
		t.Fatal("GetPackageIDsByNames must resolve package names to ids in its own statement")
	}
}
