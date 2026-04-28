//go:build !darwin

package repositories

import (
    "fmt"
    "patchmon-agent/pkg/models"
)

func CollectRepositories() ([]models.Repository, error) {
    return nil, fmt.Errorf("brew repository collection unsupported on non-darwin platforms")
}
