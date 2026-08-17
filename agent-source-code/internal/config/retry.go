package config

import "time"

// Sharing violations clear in milliseconds. The budget is generous enough to
// ride out an antivirus scan of a freshly written file without making a real
// failure take noticeably longer to surface.
const (
	fileRetryAttempts = 10
	fileRetryBackoff  = 20 * time.Millisecond
)

// retryTransientFile runs op, retrying only while it fails with an error that
// clears on its own. Anything else is returned immediately, so a genuine
// permission problem or a missing directory still fails fast.
//
// Safe to wrap a rename in: a rename that fails this way has not touched the
// destination, so retrying cannot leave a partially replaced file behind.
func retryTransientFile(op func() error) error {
	err := op()
	for attempt := 1; attempt < fileRetryAttempts && isTransientFileError(err); attempt++ {
		time.Sleep(fileRetryBackoff)
		err = op()
	}
	return err
}
