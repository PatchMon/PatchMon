package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type pruneCall struct {
	table  string
	hostID string
	ids    []string
}

type recordingDBTX struct {
	calls []pruneCall
}

func (r *recordingDBTX) Exec(_ context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "DELETE FROM") && strings.Contains(sql, "NOT (") {
		table := strings.Fields(sql[strings.Index(sql, "DELETE FROM")+len("DELETE FROM"):])[0]
		r.calls = append(r.calls, pruneCall{table: table, hostID: args[0].(string), ids: args[1].([]string)})
	}
	return pgconn.NewCommandTag("DELETE 0"), nil
}

type emptyRows struct{}

func (emptyRows) Close()                                       {}
func (emptyRows) Err() error                                   { return nil }
func (emptyRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT 0") }
func (emptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (emptyRows) Next() bool                                   { return false }
func (emptyRows) Scan(...any) error                            { return errors.New("no rows") }
func (emptyRows) Values() ([]any, error)                       { return nil, nil }
func (emptyRows) RawValues() [][]byte                          { return nil }
func (emptyRows) Conn() *pgx.Conn                              { return nil }

func (r *recordingDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return emptyRows{}, nil
}

func (r *recordingDBTX) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return nil
}

func TestReceiveDockerDataPruneDecision(t *testing.T) {
	containers := []DockerReceiveContainer{{ContainerID: "c1", Status: "running"}}
	volumes := []DockerReceiveVolume{{VolumeID: "v1"}}
	networks := []DockerReceiveNetwork{{NetworkID: "n1"}}

	cases := []struct {
		name    string
		payload DockerReceivePayload
		want    []pruneCall
	}{
		{
			name:    "populated lists prune without collected",
			payload: DockerReceivePayload{Containers: containers, Volumes: volumes, Networks: networks},
			want: []pruneCall{
				{"docker_containers", "h1", []string{"c1"}},
				{"docker_volumes", "h1", []string{"v1"}},
				{"docker_networks", "h1", []string{"n1"}},
			},
		},
		{
			name:    "empty lists from an agent without collected do not prune",
			payload: DockerReceivePayload{},
			want:    nil,
		},
		{
			name:    "empty successful collection prunes",
			payload: DockerReceivePayload{Collected: []string{"containers", "images", "volumes", "networks"}},
			want: []pruneCall{
				{"docker_host_images", "h1", []string{}},
				{"docker_containers", "h1", []string{}},
				{"docker_volumes", "h1", []string{}},
				{"docker_networks", "h1", []string{}},
			},
		},
		{
			name:    "failed kinds are left alone",
			payload: DockerReceivePayload{Networks: networks, Collected: []string{"images", "networks"}},
			want:    []pruneCall{{"docker_host_images", "h1", []string{}}, {"docker_networks", "h1", []string{"n1"}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingDBTX{}
			p := tc.payload
			if _, err := receiveDockerData(context.Background(), db.New(rec), "h1", &p, time.Now()); err != nil {
				t.Fatal(err)
			}
			if len(rec.calls) != len(tc.want) {
				t.Fatalf("got %d prune calls %+v, want %+v", len(rec.calls), rec.calls, tc.want)
			}
			for i, w := range tc.want {
				g := rec.calls[i]
				if g.table != w.table || g.hostID != w.hostID || !slices.Equal(g.ids, w.ids) || g.ids == nil {
					t.Errorf("call %d: got %+v, want %+v", i, g, w)
				}
			}
		})
	}
}

func TestReceiveDockerDataPrunesAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)

	exec := func(sql string, args ...interface{}) {
		t.Helper()
		tx, err := d.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(table, col, hostID string) []string {
		t.Helper()
		tx, err := d.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		rows, err := tx.Query(ctx, "SELECT "+col+" FROM "+table+" WHERE host_id = $1 ORDER BY 1", hostID)
		if err != nil {
			t.Fatal(err)
		}
		out, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	run := uuid.NewString()
	repo := "prune-test-" + run
	h1, h2 := "h1-"+run, "h2-"+run
	for _, h := range []string{h1, h2} {
		exec(`INSERT INTO hosts (id, friendly_name, os_type, os_version, updated_at, api_id, api_key)
			VALUES ($1, $1, 'linux', '1', now(), $1, $1)`, h)
	}
	t.Cleanup(func() {
		exec(`DELETE FROM hosts WHERE id = ANY($1)`, []string{h1, h2})
		exec(`DELETE FROM docker_images WHERE repository = $1`, repo)
	})

	s := NewDockerStore(d)
	receive := func(hostID string, p DockerReceivePayload) {
		t.Helper()
		if _, err := s.ReceiveDockerData(ctx, hostID, &p); err != nil {
			t.Fatal(err)
		}
	}
	container := func(id string) DockerReceiveContainer {
		return DockerReceiveContainer{ContainerID: id, Name: id, ImageRepository: repo, ImageTag: "1", ImageID: "sha-" + id, Status: "running"}
	}
	all := []string{"containers", "images", "volumes", "networks"}
	full := DockerReceivePayload{
		Containers: []DockerReceiveContainer{container("cA"), container("cB")},
		Volumes:    []DockerReceiveVolume{{VolumeID: "vA", Name: "vA"}, {VolumeID: "vB", Name: "vB"}},
		Networks:   []DockerReceiveNetwork{{NetworkID: "nA", Name: "nA"}, {NetworkID: "nB", Name: "nB"}},
		Collected:  all,
	}
	check := func(step, hostID string, wantC, wantV, wantN []string) {
		t.Helper()
		for _, c := range []struct {
			table, col string
			want       []string
		}{
			{"docker_containers", "container_id", wantC},
			{"docker_volumes", "volume_id", wantV},
			{"docker_networks", "network_id", wantN},
		} {
			if got := ids(c.table, c.col, hostID); !slices.Equal(got, c.want) {
				t.Errorf("%s: %s on %s = %v, want %v", step, c.table, hostID, got, c.want)
			}
		}
	}
	both := []string{"cA", "cB"}

	receive(h1, full)
	receive(h2, full)

	receive(h1, DockerReceivePayload{
		Containers: []DockerReceiveContainer{container("cB")},
		Volumes:    []DockerReceiveVolume{{VolumeID: "vB", Name: "vB"}},
		Networks:   []DockerReceiveNetwork{{NetworkID: "nB", Name: "nB"}},
	})
	check("removed entries", h1, []string{"cB"}, []string{"vB"}, []string{"nB"})
	check("other host", h2, both, []string{"vA", "vB"}, []string{"nA", "nB"})

	receive(h1, DockerReceivePayload{})
	check("old agent empty report", h1, []string{"cB"}, []string{"vB"}, []string{"nB"})

	receive(h1, DockerReceivePayload{Collected: []string{"images", "networks"}, Networks: []DockerReceiveNetwork{{NetworkID: "nB", Name: "nB"}}})
	check("failed collection", h1, []string{"cB"}, []string{"vB"}, []string{"nB"})

	receive(h1, DockerReceivePayload{Collected: all})
	check("empty successful collection", h1, nil, nil, nil)
	check("other host after empty", h2, both, []string{"vA", "vB"}, []string{"nA", "nB"})
}
