// Package buildinfo holds version metadata injected at link time:
//
//	-ldflags "-X github.com/prasadgujar22/shorty/pkg/buildinfo.Version=$(git describe)"
package buildinfo

import (
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	Version = "dev"
	Commit  = "unknown"
)

// Register exposes shorty_build_info{version,commit,goversion} = 1.
func Register(reg prometheus.Registerer) {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shorty_build_info",
		Help: "Build metadata; value is always 1.",
	}, []string{"version", "commit", "goversion"})
	g.WithLabelValues(Version, Commit, runtime.Version()).Set(1)
	reg.MustRegister(g)
}
