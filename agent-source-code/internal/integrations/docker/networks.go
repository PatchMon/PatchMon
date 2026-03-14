package docker

import (
	"context"
	"fmt"

	"patchmon-agent/pkg/models"

	"github.com/docker/docker/api/types/network"
)

// collectNetworks collects all Docker networks
func (d *Integration) collectNetworks(ctx context.Context) ([]models.DockerNetwork, error) {
	// List all networks
	networks, err := d.client.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	result := make([]models.DockerNetwork, 0, len(networks))

	for _, net := range networks {
		// Parse IPAM configuration
		var ipam *models.DockerIPAM
		// Check if IPAM config exists (Config slice length > 0 or Driver is set)
		if len(net.IPAM.Config) > 0 || net.IPAM.Driver != "" {
			ipam = &models.DockerIPAM{
				Driver:  net.IPAM.Driver,
				Options: net.IPAM.Options,
				Config:  make([]models.DockerIPAMConfig, 0),
			}

			for _, ipamConfig := range net.IPAM.Config {
				// Convert auxiliary addresses to map
				auxAddresses := make(map[string]string)
				if ipamConfig.AuxAddress != nil {
					for k, v := range ipamConfig.AuxAddress {
						auxAddresses[k] = v
					}
				}

				ipamConfigData := models.DockerIPAMConfig{
					Subnet:       ipamConfig.Subnet,
					Gateway:      ipamConfig.Gateway,
					IPRange:      ipamConfig.IPRange,
					AuxAddresses: auxAddresses,
				}
				ipam.Config = append(ipam.Config, ipamConfigData)
			}
		}

		// Count containers attached to this network
		containerCount := len(net.Containers)

		networkData := models.DockerNetwork{
			NetworkID:      net.ID,
			Name:           net.Name,
			Driver:         net.Driver,
			Scope:          net.Scope,
			IPv6Enabled:    net.EnableIPv6,
			Internal:       net.Internal,
			Attachable:     net.Attachable,
			Ingress:        net.Ingress,
			ConfigOnly:     net.ConfigOnly,
			Labels:         net.Labels,
			IPAM:           ipam,
			ContainerCount: containerCount,
		}

		// Note: Docker networks don't have a CreatedAt timestamp in the List response
		// We'd need to inspect each network individually to get it, which is expensive
		// So we'll leave CreatedAt as nil for now

		result = append(result, networkData)
	}

	return result, nil
}
