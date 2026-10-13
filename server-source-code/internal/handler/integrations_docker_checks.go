package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

const (
	maxImageCheckResults    = 5000
	maxImageRepositoryLen   = 512
	maxImageTagLen          = 128
	maxImageIDLen           = 128
	maxImageRegistryLen     = 255
	maxImageAgentVersionLen = 64
	maxImageCheckClockSkew  = 5 * time.Minute
	maxImageUpdateRefs      = 10
)

var imageDigestRe = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

var imageCheckStatuses = map[string]bool{
	store.ImageStatusUpToDate:        true,
	store.ImageStatusUpdateAvailable: true,
	store.ImageStatusUnknown:         true,
	store.ImageStatusLocalBuild:      true,
	store.ImageStatusPinned:          true,
	store.ImageStatusSkipped:         true,
}

var imageCheckReasons = map[string]bool{
	"":                             true,
	"rate_limited":                 true,
	"auth_failed":                  true,
	"tag_not_found":                true,
	"unreachable":                  true,
	"unsupported_auth":             true,
	"insecure_registry_disallowed": true,
	"no_digest":                    true,
}

type dockerImageCheckResultReq struct {
	Repository   string `json:"repository"`
	Tag          string `json:"tag"`
	ImageID      string `json:"image_id"`
	Status       string `json:"status"`
	LocalDigest  string `json:"local_digest"`
	RemoteDigest string `json:"remote_digest"`
	Reason       string `json:"reason"`
	Registry     string `json:"registry"`
}

type dockerImageChecksReq struct {
	CheckedAt    time.Time                   `json:"checked_at"`
	AgentVersion string                      `json:"agent_version"`
	Results      []dockerImageCheckResultReq `json:"results"`
}

func validateDockerImageChecks(p *dockerImageChecksReq) error {
	if len(p.AgentVersion) > maxImageAgentVersionLen {
		return fmt.Errorf("agent_version is too long")
	}
	if len(p.Results) > maxImageCheckResults {
		return fmt.Errorf("too many results (maximum %d)", maxImageCheckResults)
	}
	for i := range p.Results {
		r := &p.Results[i]
		r.ImageID = strings.TrimPrefix(r.ImageID, "sha256:")
		switch {
		case r.Repository == "" || len(r.Repository) > maxImageRepositoryLen:
			return fmt.Errorf("result %d: invalid repository", i)
		case r.Tag == "" || len(r.Tag) > maxImageTagLen:
			return fmt.Errorf("result %d: invalid tag", i)
		case r.ImageID == "" || len(r.ImageID) > maxImageIDLen:
			return fmt.Errorf("result %d: invalid image_id", i)
		case len(r.Registry) > maxImageRegistryLen:
			return fmt.Errorf("result %d: registry is too long", i)
		case !imageCheckStatuses[r.Status]:
			return fmt.Errorf("result %d: invalid status", i)
		case !imageCheckReasons[r.Reason]:
			return fmt.Errorf("result %d: invalid reason", i)
		case r.LocalDigest != "" && !imageDigestRe.MatchString(r.LocalDigest):
			return fmt.Errorf("result %d: invalid local_digest", i)
		case r.RemoteDigest != "" && !imageDigestRe.MatchString(r.RemoteDigest):
			return fmt.Errorf("result %d: invalid remote_digest", i)
		}
	}
	return nil
}

func imageCheckTime(checkedAt, now time.Time) time.Time {
	if checkedAt.IsZero() || checkedAt.After(now.Add(maxImageCheckClockSkew)) {
		return now
	}
	return checkedAt.UTC()
}

// ReceiveDockerImageChecks handles POST /api/v1/integrations/docker/image-checks (agent endpoint, API key auth).
func (h *IntegrationsHandler) ReceiveDockerImageChecks(w http.ResponseWriter, r *http.Request) {
	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "API credentials required"})
		return
	}
	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}
	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	var payload dockerImageChecksReq
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}
	if err := validateDockerImageChecks(&payload); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid image check payload: " + err.Error()})
		return
	}

	results := make([]store.DockerImageCheckResult, len(payload.Results))
	for i, res := range payload.Results {
		results[i] = store.DockerImageCheckResult(res)
	}
	checkedAt := imageCheckTime(payload.CheckedAt, time.Now().UTC())
	out, err := h.docker.ReceiveImageChecks(r.Context(), host.ID, checkedAt, results)
	if err != nil {
		slog.Error("failed to store docker image checks", "error", err, "host_id", host.ID)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to process Docker image checks"})
		return
	}
	slog.Info("received docker image checks from agent", "host_id", host.ID,
		"results", len(payload.Results), "stored", out.Received, "unresolved", out.Unresolved, "new_updates", len(out.NewUpdates))

	if h.notify != nil && len(out.NewUpdates) > 0 {
		hostName := host.FriendlyName
		if hostName == "" && host.Hostname != nil {
			hostName = *host.Hostname
		}
		refs := make([]string, 0, min(len(out.NewUpdates), maxImageUpdateRefs))
		for _, u := range out.NewUpdates[:min(len(out.NewUpdates), maxImageUpdateRefs)] {
			refs = append(refs, u.Repository+":"+u.Tag)
		}
		title, msg := imageUpdateEventText(hostName, refs, len(out.NewUpdates))
		h.notify.EmitEvent(r.Context(), h.db.DB(r.Context()), hostctx.TenantHostKey(r.Context()), notifications.Event{
			Type:          "container_image_update_available",
			Severity:      "informational",
			Title:         title,
			Message:       msg,
			ReferenceType: "host",
			ReferenceID:   host.ID,
			Metadata: map[string]interface{}{
				"host_id":      host.ID,
				"host_name":    hostName,
				"update_count": len(out.NewUpdates),
				"images":       refs,
			},
		})
	}

	JSON(w, http.StatusOK, map[string]int{
		"received":    out.Received,
		"new_updates": len(out.NewUpdates),
	})
}

func imageUpdateEventText(hostName string, refs []string, total int) (title, message string) {
	if total == 1 && len(refs) == 1 {
		return fmt.Sprintf("Docker image update available: %s", refs[0]),
			fmt.Sprintf("A newer image is available for \"%s\" on host \"%s\".", refs[0], hostName)
	}
	list := strings.Join(refs, ", ")
	if more := total - len(refs); more > 0 {
		list += fmt.Sprintf(" and %d more", more)
	}
	return fmt.Sprintf("%d Docker image updates available on %s", total, hostName),
		fmt.Sprintf("Newer images are available for %d images on host \"%s\": %s.", total, hostName, list)
}
