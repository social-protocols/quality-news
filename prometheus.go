package main

import (
	"log"
	"net/http"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/johnwarden/httperror/v2"
)

// Register various metrics.
// Metric name may contain labels in Prometheus format - see below.
var (
	// Register counter without labels.
	requestsTotal = metrics.NewCounter("requests_total")
)

func servePrometheusMetrics() {
	mux := http.NewServeMux()

	// Export all the registered metrics in Prometheus format at `/metrics` http path.
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.WritePrometheus(w, true)
	})

	log.Fatal(http.ListenAndServe(":9091", mux))
}

func prometheusMiddleware[P any](routeName string, h httperror.XHandler[P]) httperror.XHandlerFunc[P] {
	// Register summary with a single label.
	requestDuration := metrics.NewSummary(`requests_duration_seconds{route="` + routeName + `"}`)

	return func(w http.ResponseWriter, r *http.Request, p P) error {
		requestsTotal.Inc()
		startTime := time.Now()

		err := h.Serve(w, r, p)

		requestDuration.UpdateDuration(startTime)

		return err
	}
}
