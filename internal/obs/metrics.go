package obs

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	BuildInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "xiaozhi_build_info",
			Help: "Build information as a label",
		},
		[]string{"version", "go_version"},
	)
)
