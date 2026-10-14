package docker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"patchmon-agent/internal/utils"
	"patchmon-agent/pkg/models"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
)

const (
	dockerSocketPath = "/var/run/docker.sock"
	integrationName  = "docker"
)

// Integration implements the Integration interface for Docker
type Integration struct {
	client         *client.Client
	logger         *logrus.Logger
	monitoring     bool
	monitoringMu   sync.RWMutex
	stopMonitoring context.CancelFunc
}

// New creates a new Docker integration
func New(logger *logrus.Logger) *Integration {
	return &Integration{
		logger: logger,
	}
}

// Name returns the integration name
func (d *Integration) Name() string {
	return integrationName
}

// Priority returns the collection priority
func (d *Integration) Priority() int {
	return 10 // Lower priority than system collection
}

// SupportsRealtime indicates Docker supports real-time monitoring
func (d *Integration) SupportsRealtime() bool {
	return true
}

// IsAvailable checks if Docker is available on this system
func (d *Integration) IsAvailable() bool {
	// Check if Docker socket exists
	if _, err := os.Stat(dockerSocketPath); os.IsNotExist(err) {
		d.logger.Debug("Docker socket not found")
		return false
	}

	// Try to create a Docker client and ping the daemon
	cli, err := client.New(client.FromEnv)
	if err != nil {
		d.logger.WithError(err).Debug("Failed to create Docker client")
		return false
	}

	// Defer close to ensure cleanup if we don't store the client
	shouldClose := true
	defer func() {
		if shouldClose && cli != nil {
			_ = cli.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := cli.Ping(ctx, client.PingOptions{}); err != nil {
		d.logger.WithError(err).Debug("Failed to ping Docker daemon")
		return false
	}

	// Store the client for later use (prevent deferred close)
	shouldClose = false
	d.client = cli
	return true
}

// Collect gathers Docker data
func (d *Integration) Collect(ctx context.Context) (*models.IntegrationData, error) {
	startTime := time.Now()

	if d.client == nil {
		if !d.IsAvailable() {
			return nil, fmt.Errorf("docker is not available")
		}
	}

	d.logger.Debug("Collecting Docker data...")

	dockerData := &models.DockerData{
		Containers: make([]models.DockerContainer, 0),
		Images:     make([]models.DockerImage, 0),
		Volumes:    make([]models.DockerVolume, 0),
		Networks:   make([]models.DockerNetwork, 0),
		Updates:    make([]models.DockerImageUpdate, 0),
	}

	var idx *imageIndex
	images, err := d.listImages(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to collect images")
	} else {
		idx = newImageIndex(images)
		dockerData.Images = buildImages(images)
		dockerData.Collected = append(dockerData.Collected, "images")
	}

	containers, usage, err := d.collectContainers(ctx, idx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to collect containers")
	} else {
		dockerData.Containers = containers
		dockerData.Collected = append(dockerData.Collected, "containers")
	}

	volumes, err := d.collectVolumes(ctx, usage)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to collect volumes")
	} else {
		dockerData.Volumes = volumes
		dockerData.Collected = append(dockerData.Collected, "volumes")
	}

	networks, err := d.collectNetworks(ctx, usage)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to collect networks")
	} else {
		dockerData.Networks = networks
		dockerData.Collected = append(dockerData.Collected, "networks")
	}

	daemonInfo, err := d.collectDaemonInfo(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to collect daemon info")
	} else {
		dockerData.DaemonInfo = daemonInfo
	}

	d.logger.WithFields(logrus.Fields{
		"containers": len(dockerData.Containers),
		"images":     len(dockerData.Images),
		"volumes":    len(dockerData.Volumes),
		"networks":   len(dockerData.Networks),
	}).Debug("Collected Docker data")

	executionTime := time.Since(startTime).Seconds()

	return &models.IntegrationData{
		Name:          d.Name(),
		Enabled:       true,
		Data:          dockerData,
		CollectedAt:   utils.GetCurrentTimeUTC(),
		ExecutionTime: executionTime,
	}, nil
}

// Close closes the Docker client
func (d *Integration) Close() error {
	if d.client != nil {
		return d.client.Close()
	}
	return nil
}

// collectDaemonInfo collects Docker daemon information
func (d *Integration) collectDaemonInfo(ctx context.Context) (*models.DockerDaemonInfo, error) {
	infoResult, err := d.client.Info(ctx, client.InfoOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get daemon info: %w", err)
	}

	version, err := d.client.ServerVersion(ctx, client.ServerVersionOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get server version: %w", err)
	}

	return &models.DockerDaemonInfo{
		Version:       version.Version,
		APIVersion:    version.APIVersion,
		OS:            infoResult.Info.OperatingSystem,
		Architecture:  infoResult.Info.Architecture,
		KernelVersion: infoResult.Info.KernelVersion,
		TotalMemory:   infoResult.Info.MemTotal,
		NCPU:          infoResult.Info.NCPU,
	}, nil
}

// normalizeStatus converts Docker status to normalized status string
func normalizeStatus(_ string, state string) string {
	// State is more reliable than Status for determining actual state
	switch state {
	case "running":
		return "running"
	case "exited":
		return "exited"
	case "created":
		return "created"
	case "restarting":
		return "restarting"
	case "paused":
		return "paused"
	case "dead":
		return "dead"
	case "removing":
		return "removing"
	default:
		return "unknown"
	}
}

// determineImageSource determines the source registry of an image
func determineImageSource(imageName string) string {
	if len(imageName) == 0 {
		return "unknown"
	}

	// Extract domain from image name
	parts := strings.SplitN(imageName, "/", 2)
	if len(parts) == 1 {
		// No domain specified, it's Docker Hub (implicit docker.io)
		return "docker-hub"
	}

	domain := parts[0]

	// Check if first part contains a dot or colon (indicates it's a domain)
	if !strings.Contains(domain, ".") && !strings.Contains(domain, ":") {
		// It's a Docker Hub image with org/repo format (e.g., "library/nginx")
		return "docker-hub"
	}

	// Match known registry domains (inspired by diun)
	switch {
	case domain == "docker.io":
		return "docker-hub"
	case domain == "ghcr.io":
		return "github"
	case domain == "docker.pkg.github.com":
		return "github"
	case domain == "registry.gitlab.com":
		return "gitlab"
	case strings.HasPrefix(domain, "gcr.io"):
		return "google"
	case strings.Contains(domain, "pkg.dev"): // Google Artifact Registry
		return "google"
	case domain == "quay.io":
		return "quay"
	case domain == "registry.access.redhat.com":
		return "redhat"
	case strings.Contains(domain, "azurecr.io"):
		return "azure"
	case strings.Contains(domain, "amazonaws.com"): // ECR
		return "aws"
	default:
		// Private registry
		return "private"
	}
}

// convertPorts reports one binding per port/proto, preferring IPv4 over IPv6.
func convertPorts(ports []container.PortSummary) map[string]string {
	portMap := make(map[string]string)
	isV4 := make(map[string]bool)
	for _, port := range ports {
		if port.PublicPort == 0 {
			continue
		}
		key := fmt.Sprintf("%d/%s", port.PrivatePort, port.Type)
		addr := port.IP.Unmap()
		v4 := addr.Is4()
		if _, exists := portMap[key]; exists && (isV4[key] || !v4) {
			continue
		}
		switch {
		case v4:
			portMap[key] = fmt.Sprintf("%s:%d", addr, port.PublicPort)
		case addr.IsValid():
			portMap[key] = fmt.Sprintf("[%s]:%d", addr, port.PublicPort)
		default:
			portMap[key] = fmt.Sprintf("%d", port.PublicPort)
		}
		isV4[key] = v4
	}
	return portMap
}
