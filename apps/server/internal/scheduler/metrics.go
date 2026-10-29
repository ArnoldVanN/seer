package scheduler

import "github.com/prometheus/client_golang/prometheus"

// Endpoint metrics are exported by the worker that owns the endpoint, so
// scrape every worker pod to get all of them.
var (
	endpointUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "seer_endpoint_up",
		Help: "1 if the endpoint's last check succeeded, 0 otherwise.",
	}, []string{"key", "group"})
	endpointResponse = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "seer_endpoint_response_seconds",
		Help: "Response time of the endpoint's last check.",
	}, []string{"key", "group"})
	checksTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "seer_checks_total",
		Help: "Checks run by this worker.",
	}, []string{"key", "group", "success"})
	ownedEndpoints = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "seer_worker_owned_endpoints",
		Help: "Endpoints this worker currently owns.",
	})
	liveWorkers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "seer_live_workers",
		Help: "Workers this worker currently sees as live.",
	})
)

func init() {
	prometheus.MustRegister(endpointUp, endpointResponse, checksTotal, ownedEndpoints, liveWorkers)
}

func forgetMetrics(key, group string) {
	endpointUp.DeleteLabelValues(key, group)
	endpointResponse.DeleteLabelValues(key, group)
	checksTotal.DeletePartialMatch(prometheus.Labels{"key": key})
}
