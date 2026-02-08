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

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
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
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
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

	if _, err := cli.Ping(ctx); err != nil {
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

	d.logger.Info("Collecting Docker data...")

	// Collect all Docker data
	dockerData := &models.DockerData{
		Containers: make([]models.DockerContainer, 0),
		Images:     make([]models.DockerImage, 0),
		Volumes:    make([]models.DockerVolume, 0),
		Networks:   make([]models.DockerNetwork, 0),
		Updates:    make([]models.DockerImageUpdate, 0),
	}

	// Collect containers
	containers, err := d.collectContainers(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to collect containers")
	} else {
		dockerData.Containers = containers
		d.logger.WithField("count", len(containers)).Info("Collected containers")
	}

	// Collect images
	images, err := d.collectImages(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to collect images")
	} else {
		dockerData.Images = images
		d.logger.WithField("count", len(images)).Info("Collected images")
	}

	// Collect volumes
	volumes, err := d.collectVolumes(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to collect volumes")
	} else {
		dockerData.Volumes = volumes
		d.logger.WithField("count", len(volumes)).Info("Collected volumes")
	}

	// Collect networks
	networks, err := d.collectNetworks(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to collect networks")
	} else {
		dockerData.Networks = networks
		d.logger.WithField("count", len(networks)).Info("Collected networks")
	}

	// Collect daemon info
	daemonInfo, err := d.collectDaemonInfo(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to collect daemon info")
	} else {
		dockerData.DaemonInfo = daemonInfo
	}

	// Check for updates (optional, can be slow)
	// TODO: Make this configurable or run in background
	// updates, err := d.checkImageUpdates(ctx, images)
	// if err != nil {
	// 	d.logger.WithError(err).Warn("Failed to check for image updates")
	// } else {
	// 	dockerData.Updates = updates
	// 	d.logger.WithField("count", len(updates)).Info("Found image updates")
	// }

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
	info, err := d.client.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get daemon info: %w", err)
	}

	version, err := d.client.ServerVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get server version: %w", err)
	}

	return &models.DockerDaemonInfo{
		Version:       version.Version,
		APIVersion:    version.APIVersion,
		OS:            info.OperatingSystem,
		Architecture:  info.Architecture,
		KernelVersion: info.KernelVersion,
		TotalMemory:   info.MemTotal,
		NCPU:          info.NCPU,
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

// parseImageName parses image name into repository and tag
func parseImageName(fullImage string) (repository, tag string) {
	// Default tag
	tag = "latest"

	// Find the last colon to separate tag
	lastColon := -1
	for i := len(fullImage) - 1; i >= 0; i-- {
		if fullImage[i] == ':' {
			lastColon = i
			break
		}
		// If we hit a slash before a colon, there's no tag
		if fullImage[i] == '/' {
			break
		}
	}

	if lastColon > 0 {
		repository = fullImage[:lastColon]
		tag = fullImage[lastColon+1:]
	} else {
		repository = fullImage
	}

	return repository, tag
}

// cleanImageRepository removes registry prefix for common registries
func cleanImageRepository(repository string) string {
	// Remove common registry prefixes
	prefixes := []string{
		"ghcr.io/",
		"registry.gitlab.com/",
		"gcr.io/",
		"quay.io/",
	}

	for _, prefix := range prefixes {
		if len(repository) > len(prefix) && repository[:len(prefix)] == prefix {
			return repository[len(prefix):]
		}
	}

	return repository
}

// convertPorts converts Docker port bindings to simplified map
func convertPorts(ports []container.Port) map[string]string {
	portMap := make(map[string]string)
	for _, port := range ports {
		if port.PublicPort > 0 {
			key := fmt.Sprintf("%d/%s", port.PrivatePort, port.Type)
			value := fmt.Sprintf("%s:%d", port.IP, port.PublicPort)
			portMap[key] = value
		}
	}
	return portMap
}
