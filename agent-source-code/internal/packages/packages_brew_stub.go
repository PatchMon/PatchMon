//go:build !darwin

package packages

import (
    "fmt"
    "patchmon-agent/pkg/models"
)

func CollectPackages() ([]models.Package, error) {
    return nil, fmt.Errorf("brew package collection unsupported on non-darwin platforms")
}
