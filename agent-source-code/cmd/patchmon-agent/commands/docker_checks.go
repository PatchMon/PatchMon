package commands

import (
	"context"
	"errors"
	"time"

	"patchmon-agent/internal/client"
	"patchmon-agent/internal/integrations/docker"
	"patchmon-agent/internal/pkgversion"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

const (
	dockerEventDebounce        = 5 * time.Second
	dockerImageCheckFirstDelay = 2 * time.Minute
	dockerImageCheckTimeout    = 30 * time.Minute

	dockerImageCheckServerCooldown = 10 * time.Minute
)

var dockerImageCheck = docker.NewSingleFlight(runDockerImageCheck)

func consumeDockerEvents(ctx context.Context, events <-chan interface{}) {
	debounce := docker.NewDebouncer(dockerEventDebounce, func() { uploadDockerInventory(ctx) })
	defer debounce.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if _, isStatus := ev.(models.DockerStatusEvent); isStatus {
				debounce.Trigger()
			}
		}
	}
}

func startDockerImageChecks(ctx context.Context) {
	go dockerImageCheck.RunEvery(ctx, dockerImageCheckFirstDelay, func() time.Duration {
		_, interval := cfgManager.GetDockerUpdateCheck()
		return interval
	})
}

func serverCheckTooSoon(lastFinished, now time.Time) (time.Duration, bool) {
	if lastFinished.IsZero() {
		return 0, false
	}
	since := now.Sub(lastFinished)
	return since, since < dockerImageCheckServerCooldown
}

func triggerDockerImageCheck(ctx context.Context) {
	if since, tooSoon := serverCheckTooSoon(dockerImageCheck.LastFinished(), time.Now()); tooSoon {
		logger.Infof("Docker image update check skipped, last check finished %d min ago", int(since.Minutes()))
		return
	}
	if !dockerImageCheck.Run(ctx) {
		logger.Info("Docker image update check already running")
	}
}

func runDockerImageCheck(ctx context.Context) {
	enabled, _ := cfgManager.GetDockerUpdateCheck()
	if !cfgManager.IsIntegrationEnabled("docker") || !enabled {
		logger.Debug("Docker image update check disabled, skipping")
		return
	}

	ctx, cancel := context.WithTimeout(ctx, dockerImageCheckTimeout)
	defer cancel()

	d := docker.New(logger)
	defer func() { _ = d.Close() }()
	payload, err := d.CheckImageUpdates(ctx, pkgversion.Version)
	if errors.Is(err, docker.ErrDockerUnavailable) {
		logger.Debug("Docker is not available, skipping image update check")
		return
	}
	if err != nil {
		logger.WithError(err).Warn("Docker image update check failed")
		return
	}

	sendCtx, sendCancel := context.WithTimeout(ctx, 30*time.Second)
	defer sendCancel()
	resp, err := client.New(cfgManager, logger).SendDockerImageChecks(sendCtx, payload)
	if err != nil {
		logger.WithError(err).Warn("Failed to send Docker image update check results")
		return
	}
	logger.WithFields(logrus.Fields{
		"received":    resp.Received,
		"new_updates": resp.NewUpdates,
	}).Info("Docker image update check results sent")
}
