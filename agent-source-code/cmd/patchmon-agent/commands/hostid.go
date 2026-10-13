package commands

import (
	"sync"

	"patchmon-agent/internal/config"

	"github.com/sirupsen/logrus"
)

var hostIDConflictOnce sync.Once

func reconcileHostID(serverID string) {
	result, err := cfgManager.AdoptHostID(serverID)
	if err != nil {
		logger.WithError(err).Warn("Could not save the host_id learned from the server")
		return
	}
	switch result {
	case config.HostIDAdopted:
		logger.WithField("host_id", serverID).Info("Learned host_id from the server")
	case config.HostIDConflict:
		hostIDConflictOnce.Do(func() {
			logger.WithFields(logrus.Fields{
				"config_host_id": cfgManager.GetHostID(),
				"server_host_id": serverID,
			}).Warn("config.yml names a different host than these credentials belong to; keeping it. Run 'patchmon-agent config sync-host-id' to adopt the server's host ID, or reinstall the agent")
		})
	}
}
