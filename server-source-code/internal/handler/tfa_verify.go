package handler

import (
	"context"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

// verifyTfaCode accepts a TOTP code once per step or consumes a backup code atomically.
func verifyTfaCode(ctx context.Context, users *store.UsersStore, replay *store.TfaReplayStore, user *models.User, token string) (bool, error) {
	token = strings.ToUpper(strings.TrimSpace(token))
	if len(token) != 6 || !util.TokenRegex.MatchString(token) {
		return false, nil
	}
	if user.TfaSecret != nil {
		if step, ok := util.MatchTOTPStep(strings.TrimSpace(*user.TfaSecret), token, util.TOTPWindow, time.Now()); ok {
			return replay.MarkUsed(ctx, user.ID, step)
		}
	}
	codes := user.TfaBackupCodes
	for range 3 {
		if codes == nil {
			return false, nil
		}
		hashed := util.ParseBackupCodesJSON(codes)
		ok, idx := util.VerifyBackupCode(token, hashed)
		if !ok {
			return false, nil
		}
		remaining := util.EncodeBackupCodesJSON(append(hashed[:idx:idx], hashed[idx+1:]...))
		consumed, err := users.ConsumeTfaBackupCode(ctx, user.ID, *codes, remaining)
		if err != nil || consumed {
			return consumed, err
		}
		fresh, err := users.GetByID(ctx, user.ID)
		if err != nil {
			return false, err
		}
		codes = fresh.TfaBackupCodes
	}
	return false, nil
}

// WithTfaReplay wires single-use TOTP enforcement.
func (h *AuthHandler) WithTfaReplay(r *store.TfaReplayStore) *AuthHandler {
	h.tfaReplay = r
	return h
}
