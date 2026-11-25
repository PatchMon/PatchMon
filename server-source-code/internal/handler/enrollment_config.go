package handler

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

func enrollmentTokenResponse(r *http.Request, item store.TokenListItem) store.TokenListItem {
	if middleware.GetApiToken(r.Context()) != nil {
		// Basic Auth clients receive enrollment credentials only when creating them.
		item.TokenKey = ""
	}
	return item
}

// Missing integration_type is supported for enrollment tokens created before
// integration types were introduced. Unknown and administrative types fail closed.
func isEnrollmentToken(metadata []byte) bool {
	meta, err := enrollmentMetadata(metadata)
	if err != nil {
		return false
	}
	for key := range meta {
		// Auth decodes JSON keys without case sensitivity. Reject aliases of this
		// reserved key so classification and authentication cannot disagree.
		if strings.EqualFold(key, "integration_type") && key != "integration_type" {
			return false
		}
	}
	value, exists := meta["integration_type"]
	if !exists {
		return true
	}
	integrationType, ok := value.(string)
	return ok && (integrationType == "proxmox-lxc" || integrationType == "direct-host")
}

func allowEnrollmentTokenManagement(w http.ResponseWriter, r *http.Request, metadata []byte) bool {
	if middleware.GetApiToken(r.Context()) != nil && !isEnrollmentToken(metadata) {
		Error(w, http.StatusForbidden, "Scoped credentials can only manage enrollment tokens")
		return false
	}
	return true
}

func enrollmentMetadata(raw json.RawMessage) (map[string]interface{}, error) {
	metadata := map[string]interface{}{}
	if len(raw) == 0 || string(raw) == "null" {
		return metadata, nil
	}
	// Preserve large integers used as external identifiers when metadata is merged.
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&metadata); err != nil || metadata == nil {
		return nil, errors.New("Metadata must be an object")
	}
	return metadata, nil
}

func mergeEnrollmentMetadata(existing, patch json.RawMessage) ([]byte, error) {
	metadata, err := enrollmentMetadata(existing)
	if err != nil {
		return nil, err
	}
	updates, err := enrollmentMetadata(patch)
	if err != nil {
		return nil, err
	}
	for key, value := range updates {
		if strings.EqualFold(key, "integration_type") {
			previous, previousOK := metadata[key].(string)
			next, nextOK := value.(string)
			if key != "integration_type" || !previousOK || !nextOK || previous != next {
				return nil, errors.New("Integration type cannot be changed")
			}
		}
		metadata[key] = value
	}
	return json.Marshal(metadata)
}

func validateEnrollmentConfig(maxHosts *int32, ranges []string) error {
	if maxHosts != nil && (*maxHosts < 1 || *maxHosts > 1000) {
		return errors.New("Max hosts per day must be between 1 and 1000")
	}
	for _, value := range ranges {
		value = strings.TrimSpace(value)
		if net.ParseIP(value) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(value); err != nil {
			return errors.New("Allowed IP ranges must contain valid IP addresses or CIDR ranges")
		}
	}
	return nil
}

func parseEnrollmentExpiration(value *string) (pgtype.Timestamp, error) {
	if value == nil || *value == "" {
		return pgtype.Timestamp{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04"} {
		if parsed, err := time.Parse(layout, *value); err == nil {
			return pgtime.From(parsed), nil
		}
	}
	return pgtype.Timestamp{}, errors.New("Expiration must be a valid RFC3339 or datetime-local timestamp")
}
