//go:build !darwin

package system

func (d *Detector) getDarwinOSInfo() (osType, osVersion string, err error) {
	return "Unknown", "Unknown", nil
}

func (d *Detector) checkDarwinRebootRequired() (bool, string) {
	return false, ""
}

func (d *Detector) getDarwinKernelVersion() string {
	return "Unknown"
}
