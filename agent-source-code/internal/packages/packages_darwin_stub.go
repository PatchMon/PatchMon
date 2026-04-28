//go:build !darwin

package packages

import (
	"fmt"
	"patchmon-agent/pkg/models"
)

func CollectDarwinPackages() ([]models.Package, error) {
	return nil, fmt.Errorf("darwin package collection not supported on this platform")
}
