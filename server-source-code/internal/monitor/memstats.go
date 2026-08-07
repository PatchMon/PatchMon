package monitor

import (
	"context"
	"runtime"
	"time"

	"log/slog"
)

// StartMemStats logs runtime.MemStats periodically until ctx is done.
// Call with a background goroutine when pprof is enabled for ongoing memory monitoring.
func StartMemStats(ctx context.Context, log *slog.Logger, intervalSec int) {
	if intervalSec <= 0 {
		intervalSec = 60
	}
	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			log.Info("memstats",
				"alloc_mb", m.Alloc/1024/1024,
				"total_alloc_mb", m.TotalAlloc/1024/1024,
				"sys_mb", m.Sys/1024/1024,
				"num_gc", m.NumGC,
				"heap_alloc_mb", m.HeapAlloc/1024/1024,
				"heap_sys_mb", m.HeapSys/1024/1024,
			)
		}
	}
}
