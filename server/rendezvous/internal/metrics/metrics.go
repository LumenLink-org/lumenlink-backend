package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	AttestationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "lumenlink_attestation_total",
			Help: "Total attestation verification attempts",
		},
		[]string{"platform", "result"},
	)
	AttestationFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "lumenlink_attestation_failures_total",
			Help: "Attestation failures by reason",
		},
		[]string{"platform", "reason"},
	)
	ConfigPackGenerated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "lumenlink_config_pack_generated_total",
			Help: "Config packs generated",
		},
		[]string{"region"},
	)
	GatewayStatusUpdates = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "lumenlink_gateway_status_updates_total",
			Help: "Gateway status updates received",
		},
	)
	DiscoveryLogs = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "lumenlink_discovery_logs_total",
			Help: "Discovery log entries",
		},
		[]string{"channel", "success"},
	)
)

func init() {
	prometheus.MustRegister(
		AttestationTotal,
		AttestationFailures,
		ConfigPackGenerated,
		GatewayStatusUpdates,
		DiscoveryLogs,
	)
}
