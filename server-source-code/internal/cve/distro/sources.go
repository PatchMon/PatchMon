package distro

// DefaultSources returns the live per-distribution CVE sources, one per
// supported distro family. This is the single registration point: adding a new
// distro means adding its Source here. Proxmox derives its verdicts from the
// Ubuntu source, so it shares the same instance (and its cache).
func DefaultSources() []Source {
	ubuntu := NewUbuntu()
	return []Source{
		ubuntu,
		NewDebian(),
		NewRHEL(),
		NewFedora(),
		NewCentOS(),
		NewProxmox(ubuntu),
	}
}
