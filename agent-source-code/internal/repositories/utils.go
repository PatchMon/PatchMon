package repositories

import (
	"fmt"
	"strings"
)

// generateRepoName generates a meaningful repository name from URL and components
func generateRepoName(url, distribution, components string) string {
	// Extract domain from URL
	parts := strings.Split(url, "/")
	if len(parts) < 3 {
		return distribution // fallback
	}

	domain := strings.Split(parts[2], ":")[0] // Remove port if present

	// Remove common prefixes and suffixes to clean up domain
	domain = strings.TrimPrefix(domain, "www.")
	domain = strings.TrimSuffix(domain, ".com")
	domain = strings.TrimSuffix(domain, ".org")
	domain = strings.TrimSuffix(domain, ".net")

	// Build base name from domain and distribution
	baseName := fmt.Sprintf("%s-%s", domain, distribution)

	// Add component-specific suffixes for common cases
	if components != "" {
		if strings.Contains(components, "security") && !strings.Contains(baseName, "security") {
			baseName += "-security"
		} else if strings.Contains(components, "updates") && !strings.Contains(baseName, "updates") {
			baseName += "-updates"
		} else if strings.Contains(components, "backports") && !strings.Contains(baseName, "backports") {
			baseName += "-backports"
		} else if strings.Contains(components, "main") && strings.Contains(components, " ") {
			// Multiple components, don't add suffix
		} else if components != "main" && !strings.Contains(components, " ") {
			// Single non-main component
			baseName += fmt.Sprintf("-%s", components)
		}
	}

	// Clean up the name
	baseName = strings.ReplaceAll(baseName, ".", "-")
	baseName = strings.ToLower(baseName)

	return baseName
}
