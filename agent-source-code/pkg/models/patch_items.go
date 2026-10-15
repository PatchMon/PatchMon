package models

// PatchRunItem is one Windows Update or winget app result posted to /patching/runs/{id}/items.
type PatchRunItem struct {
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	Revision       int32  `json:"revision,omitempty"`
	Title          string `json:"title,omitempty"`
	KB             string `json:"kb,omitempty"`
	Status         string `json:"status"`
	ResultCode     *int32 `json:"result_code,omitempty"`
	HResult        string `json:"hresult,omitempty"`
	ExitCode       *int64 `json:"exit_code,omitempty"`
	RebootRequired bool   `json:"reboot_required"`
	DownloadBytes  *int64 `json:"download_bytes,omitempty"`
	Message        string `json:"message,omitempty"`
	StartedAt      string `json:"started_at,omitempty"`
	CompletedAt    string `json:"completed_at,omitempty"`
}
