package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

type fakeEnrollHosts struct {
	existing  *models.Host
	lookupErr error
	lookups   []string
	created   []*models.Host
}

func (f *fakeEnrollHosts) Count(context.Context) (int, error) { return len(f.created), nil }

func (f *fakeEnrollHosts) GetByMachineIDAndName(_ context.Context, machineID, friendlyName string) (*models.Host, error) {
	f.lookups = append(f.lookups, machineID+"/"+friendlyName)
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	if f.existing == nil || f.existing.MachineID == nil || *f.existing.MachineID != machineID || !strings.EqualFold(f.existing.FriendlyName, friendlyName) {
		return nil, nil
	}
	return f.existing, nil
}

func (f *fakeEnrollHosts) Create(_ context.Context, h *models.Host) error {
	f.created = append(f.created, h)
	return nil
}

func (f *fakeEnrollHosts) SetHostGroups(context.Context, string, []string) error { return nil }

func (f *fakeEnrollHosts) GetByID(context.Context, string) (*models.Host, error) { return nil, nil }

func (f *fakeEnrollHosts) ReissueApiKey(context.Context, string, string) (bool, error) {
	return false, nil
}

func TestCreateEnrolledHost_DuplicateGuard(t *testing.T) {
	sharedID := "lxc-100-abc"
	tests := []struct {
		name          string
		machineID     string
		friendlyName  string
		existing      *models.Host
		lookupErr     error
		wantStatus    int
		wantBody      string
		wantLookups   int
		wantCreated   int
		wantMachineID string
	}{
		{
			name:         "same machine id and name gives 409 and creates nothing",
			machineID:    sharedID,
			friendlyName: "web-01",
			existing:     &models.Host{ID: "host-1", MachineID: &sharedID, FriendlyName: "web-01"},
			wantStatus:   http.StatusConflict,
			wantBody:     `{"code":"machine_id_match","error":"Host already enrolled","friendly_name":"web-01"}`,
			wantLookups:  1,
		},
		{
			name:         "name match is case-insensitive",
			machineID:    sharedID,
			friendlyName: "WEB-01",
			existing:     &models.Host{ID: "host-1", MachineID: &sharedID, FriendlyName: "web-01"},
			wantStatus:   http.StatusConflict,
			wantBody:     `{"code":"machine_id_match","error":"Host already enrolled","friendly_name":"web-01"}`,
			wantLookups:  1,
		},
		{
			name:          "same machine id with a different name is created",
			machineID:     sharedID,
			friendlyName:  "web-02",
			existing:      &models.Host{ID: "host-1", MachineID: &sharedID, FriendlyName: "web-01"},
			wantLookups:   1,
			wantCreated:   1,
			wantMachineID: sharedID,
		},
		{
			name:          "unknown machine id is created",
			machineID:     "lxc-101-def",
			friendlyName:  "web-01",
			wantLookups:   1,
			wantCreated:   1,
			wantMachineID: "lxc-101-def",
		},
		{
			name:          "pending placeholder skips the lookup and is created",
			machineID:     "pending-123",
			wantCreated:   1,
			wantMachineID: "pending-123",
		},
		{
			name:        "empty machine id skips the lookup and gets a placeholder",
			wantCreated: 1,
		},
		{
			name:        "uninitialized machine id skips the lookup and gets a placeholder",
			machineID:   "uninitialized",
			wantCreated: 1,
		},
		{
			name:        "vendor default SMBIOS UUID skips the lookup and gets a placeholder",
			machineID:   "03000200-0400-0500-0006-000700080009",
			wantCreated: 1,
		},
		{
			name:        "lookup error gives 500 and creates nothing",
			machineID:   "lxc-102-ghi",
			lookupErr:   errors.New("db down"),
			wantStatus:  http.StatusInternalServerError,
			wantLookups: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeEnrollHosts{existing: tt.existing, lookupErr: tt.lookupErr}
			h := &AutoEnrollmentHandler{hosts: fake}
			rec := httptest.NewRecorder()

			friendlyName := tt.friendlyName
			if friendlyName == "" {
				friendlyName = "web-01"
			}
			host, apiKey, ok := h.createEnrolledHost(context.Background(), rec, friendlyName, tt.machineID, nil, "token-1")

			if got := len(fake.lookups); got != tt.wantLookups {
				t.Fatalf("lookups = %d, want %d", got, tt.wantLookups)
			}
			if got := len(fake.created); got != tt.wantCreated {
				t.Fatalf("Create calls = %d, want %d", got, tt.wantCreated)
			}
			if tt.wantStatus != 0 {
				if ok {
					t.Fatal("expected the enrolment to be rejected")
				}
				if rec.Code != tt.wantStatus {
					t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
				}
				if body := strings.TrimSpace(rec.Body.String()); tt.wantBody != "" && body != tt.wantBody {
					t.Fatalf("body = %q, want %q", body, tt.wantBody)
				}
				return
			}
			if !ok || host == nil || apiKey == "" {
				t.Fatalf("expected a created host, got ok=%v host=%v apiKey=%q", ok, host, apiKey)
			}
			if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
				t.Fatalf("nothing should be written on success, got %d %q", rec.Code, rec.Body.String())
			}
			if host.MachineID == nil {
				t.Fatal("created host has no machine_id")
			}
			if tt.wantMachineID != "" && *host.MachineID != tt.wantMachineID {
				t.Fatalf("machine_id = %q, want %q", *host.MachineID, tt.wantMachineID)
			}
			if tt.wantMachineID == "" && !strings.HasPrefix(*host.MachineID, "pending-") {
				t.Fatalf("machine_id = %q, want a pending- placeholder", *host.MachineID)
			}
			if strings.HasPrefix(*host.MachineID, "pending-") {
				if host.EnrolmentMachineID != nil {
					t.Fatalf("enrolment_machine_id = %q, want NULL for a placeholder", *host.EnrolmentMachineID)
				}
			} else if host.EnrolmentMachineID == nil || *host.EnrolmentMachineID != *host.MachineID {
				t.Fatalf("enrolment_machine_id = %v, want %q", host.EnrolmentMachineID, *host.MachineID)
			}
			if host.ApiKey == apiKey {
				t.Fatal("stored api_key must be the hash, not the plain key")
			}
			if host.EnrolmentTokenID == nil || *host.EnrolmentTokenID != "token-1" {
				t.Fatalf("enrolment_token_id = %v, want token-1", host.EnrolmentTokenID)
			}
		})
	}
}
