package docker

import (
	"context"
	"fmt"

	"patchmon-agent/pkg/models"

	"github.com/moby/moby/client"
)

// collectNetworks collects all Docker networks
func (d *Integration) collectNetworks(ctx context.Context, usage *containerUsage) ([]models.DockerNetwork, error) {
	// List all networks
	networkResult, err := d.client.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	result := make([]models.DockerNetwork, 0, len(networkResult.Items))

	for _, net := range networkResult.Items {
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
				// Convert auxiliary addresses to map (netip.Addr → string)
				auxAddresses := make(map[string]string)
				if ipamConfig.AuxAddress != nil {
					for k, v := range ipamConfig.AuxAddress {
						auxAddresses[k] = v.String()
					}
				}

				ipamConfigData := models.DockerIPAMConfig{AuxAddresses: auxAddresses}
				if ipamConfig.Subnet.IsValid() {
					ipamConfigData.Subnet = ipamConfig.Subnet.String()
				}
				if ipamConfig.Gateway.IsValid() {
					ipamConfigData.Gateway = ipamConfig.Gateway.String()
				}
				if ipamConfig.IPRange.IsValid() {
					ipamConfigData.IPRange = ipamConfig.IPRange.String()
				}
				ipam.Config = append(ipam.Config, ipamConfigData)
			}
		}

		networkData := models.DockerNetwork{
			NetworkID:   net.ID,
			Name:        net.Name,
			Driver:      net.Driver,
			Scope:       net.Scope,
			IPv6Enabled: net.EnableIPv6,
			Internal:    net.Internal,
			Attachable:  net.Attachable,
			Ingress:     net.Ingress,
			ConfigOnly:  net.ConfigOnly,
			Labels:      net.Labels,
			IPAM:        ipam,
		}
		if !net.Created.IsZero() {
			created := net.Created
			networkData.CreatedAt = &created
		}
		if usage != nil {
			networkData.ContainerCount = usage.networks[net.ID] + usage.networks[net.Name]
		}

		result = append(result, networkData)
	}

	return result, nil
}
