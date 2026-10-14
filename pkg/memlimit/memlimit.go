// Package memlimit sets the Go soft memory limit (GOMEMLIMIT) from the
// container memory limit.
//
// Kubernetes exposes limits.memory to the pod through the downward API; we
// read it from MEMORY_LIMIT_BYTES and set the soft limit to a ratio of it
// (default 0.9) so the GC works harder before the kernel OOM-kills the pod.
// An explicit GOMEMLIMIT always wins. GOMAXPROCS needs no help: since Go 1.25
// the runtime derives it from the cgroup CPU limit.
package memlimit

import (
	"log/slog"
	"os"
	"runtime/debug"
	"strconv"
)

// Apply sets the soft memory limit and returns the value set (0 if none).
func Apply(log *slog.Logger, ratio float64) int64 {
	if v := os.Getenv("GOMEMLIMIT"); v != "" {
		log.Info("memory limit from GOMEMLIMIT", "gomemlimit", v)
		return 0
	}
	limit, err := strconv.ParseInt(os.Getenv("MEMORY_LIMIT_BYTES"), 10, 64)
	if err != nil || limit <= 0 {
		return 0
	}
	soft := Compute(limit, ratio)
	debug.SetMemoryLimit(soft)
	log.Info("memory limit set from container limit", "container_limit_bytes", limit, "ratio", ratio, "gomemlimit_bytes", soft)
	return soft
}

// Compute returns ratio*limit, clamping ratio to (0, 1].
func Compute(limit int64, ratio float64) int64 {
	if ratio <= 0 || ratio > 1 {
		ratio = 0.9
	}
	return int64(float64(limit) * ratio)
}
