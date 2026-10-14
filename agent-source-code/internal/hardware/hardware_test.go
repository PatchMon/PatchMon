package hardware

import "testing"

func TestIsPodVolumeMount(t *testing.T) {
	cases := []struct {
		name       string
		mountpoint string
		want       bool
	}{
		{
			name:       "csi pod volume from issue 1079",
			mountpoint: "/var/lib/kubelet/pods/8f3c1e2a-4b5d-11ee-be56-0242ac120002/volumes/kubernetes.io~csi/pvc-9a7b/mount",
			want:       true,
		},
		{
			name:       "csi globalmount is the survivor",
			mountpoint: "/var/lib/kubelet/plugins/kubernetes.io/csi/driver.longhorn.io/0b1c2d3e4f5a6b7c8d9e/globalmount",
			want:       false,
		},
		{
			name:       "projected pod volume",
			mountpoint: "/var/lib/kubelet/pods/8f3c1e2a/volumes/kubernetes.io~projected/kube-api-access-x7k2p",
			want:       true,
		},
		{
			name:       "empty-dir pod volume",
			mountpoint: "/var/lib/kubelet/pods/8f3c1e2a/volumes/kubernetes.io~empty-dir/cache",
			want:       true,
		},
		{
			name:       "custom kubelet root dir still matched",
			mountpoint: "/data/kubelet/pods/8f3c1e2a/volumes/kubernetes.io~csi/pvc-9a7b/mount",
			want:       true,
		},
		{name: "root", mountpoint: "/", want: false},
		{name: "boot", mountpoint: "/boot/efi", want: false},
		{name: "home", mountpoint: "/home", want: false},
		{name: "docker overlay", mountpoint: "/var/lib/docker/overlay2/abc123/merged", want: false},
		{name: "nfs share", mountpoint: "/mnt/nfs/volumes", want: false},
		{
			name:       "user path that merely contains pods and volumes",
			mountpoint: "/srv/pods/volumes/data",
			want:       false,
		},
		{name: "empty", mountpoint: "", want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isPodVolumeMount(c.mountpoint); got != c.want {
				t.Errorf("isPodVolumeMount(%q) = %v, want %v", c.mountpoint, got, c.want)
			}
		})
	}
}

// The reported node carried 16 Longhorn volumes as 32 entries of 8520 bytes.
func TestIsPodVolumeMount_HalvesTheReportedNode(t *testing.T) {
	var mounts []string
	for _, pvc := range []string{
		"pvc-1a", "pvc-2b", "pvc-3c", "pvc-4d", "pvc-5e", "pvc-6f", "pvc-7a", "pvc-8b",
		"pvc-9c", "pvc-10d", "pvc-11e", "pvc-12f", "pvc-13a", "pvc-14b", "pvc-15c", "pvc-16d",
	} {
		mounts = append(mounts,
			"/var/lib/kubelet/plugins/kubernetes.io/csi/driver.longhorn.io/"+pvc+"hash/globalmount",
			"/var/lib/kubelet/pods/8f3c1e2a-4b5d-11ee/volumes/kubernetes.io~csi/"+pvc+"/mount",
		)
	}

	kept := 0
	for _, m := range mounts {
		if !isPodVolumeMount(m) {
			kept++
		}
	}

	if len(mounts) != 32 {
		t.Fatalf("built %d mounts, want 32", len(mounts))
	}
	if kept != 16 {
		t.Errorf("kept %d of 32 entries, want 16 (one per volume)", kept)
	}
}
