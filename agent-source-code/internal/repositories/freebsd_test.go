package repositories

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestExtractDistributionFromURL(t *testing.T) {
	logger := logrus.New()
	manager := NewFreeBSDManager(logger)

	tests := []struct {
		name             string
		url              string
		wantDistribution string
		wantComponents   string
	}{
		{
			name:             "Standard FreeBSD URL",
			url:              "https://pkg.FreeBSD.org/FreeBSD:15:amd64/quarterly",
			wantDistribution: "FreeBSD:15:amd64",
			wantComponents:   "quarterly",
		},
		{
			name:             "Latest branch",
			url:              "https://pkg.FreeBSD.org/FreeBSD:15:amd64/latest",
			wantDistribution: "FreeBSD:15:amd64",
			wantComponents:   "latest",
		},
		{
			name:             "FreeBSD 14",
			url:              "https://pkg.FreeBSD.org/FreeBSD:14:amd64/quarterly",
			wantDistribution: "FreeBSD:14:amd64",
			wantComponents:   "quarterly",
		},
		{
			name:             "HTTP URL",
			url:              "http://pkg.FreeBSD.org/FreeBSD:15:amd64/quarterly",
			wantDistribution: "FreeBSD:15:amd64",
			wantComponents:   "quarterly",
		},
		{
			name:             "Custom mirror",
			url:              "https://mirror.example.com/freebsd/FreeBSD:15:amd64/quarterly",
			wantDistribution: "FreeBSD:15:amd64",
			wantComponents:   "quarterly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			distribution, components := manager.extractDistributionFromURL(tt.url)
			if distribution != tt.wantDistribution {
				t.Errorf("extractDistributionFromURL() distribution = %q, want %q", distribution, tt.wantDistribution)
			}
			if components != tt.wantComponents {
				t.Errorf("extractDistributionFromURL() components = %q, want %q", components, tt.wantComponents)
			}
		})
	}
}
