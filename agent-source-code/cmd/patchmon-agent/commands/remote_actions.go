package commands

import "errors"

var errRemoteActionsDisabled = errors.New("remote actions are disabled in config.yml")

// msgRemoteActionsDisabled is the reason sent back to the server for a refused patch run.
const msgRemoteActionsDisabled = "Remote actions are disabled on this host (remote_actions.enabled: false in config.yml)"

// remoteActionRefused reports whether config.yml forbids the server-initiated action,
// and logs the refusal. The setting is read on every call, so an edit of config.yml
// takes effect once the agent reloads it.
func remoteActionRefused(action string) bool {
	if cfgManager == nil || cfgManager.GetRemoteActionsEnabled() {
		return false
	}
	logger.WithField("action", action).Warn("Refused: remote actions are disabled in " + cfgManager.GetConfigFile() +
		". Set remote_actions.enabled: true there to allow them; the server cannot change this setting.")
	return true
}
