package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/johnwarden/httperror"
	"golang.org/x/exp/slog"
)

// Register various metrics.
// Metric name may contain labels in Prometheus format - see below.

var (
	crawlErrorsTotal            = metrics.NewCounter(`errors_total{type="crawl"}`)
	requestErrorsTotal          = metrics.NewCounter(`errors_total{type="request"}`)
	crawlDuration               = metrics.NewHistogram("crawl_duration_seconds")
	crawlPostprocessingDuration = metrics.NewHistogram("crawl_postprocessing_duration_seconds")

	upvotesTotal     = metrics.NewCounter(`upvotes_total`)
	submissionsTotal = metrics.NewCounter(`submissions_total`)

	databaseSizeBytes            *metrics.Gauge
	databaseFragmentationPercent *metrics.Gauge
	vacuumOperationsTotal        = metrics.NewCounter(`database_vacuum_operations_total{database="frontpage"}`)
)

func servePrometheusMetrics() func(ctx context.Context) error {
	mux := http.NewServeMux()

	// Export all the registered metrics in Prometheus format at `/metrics` http path.
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.WritePrometheus(w, true)
	})

	listenAddress := os.Getenv("LISTEN_ADDRESS")

	s := &http.Server{
		Addr:    listenAddress + ":9091",
		Handler: mux,
	}

	go func() {
		LogFatal(slog.Default(), "Listen and serve prometheus", s.ListenAndServe())
	}()

	return s.Shutdown
}

func prometheusMiddleware[P any](routeName string, h httperror.XHandler[P]) httperror.XHandlerFunc[P] {
	// Register summary with a single label.
	requestDuration := metrics.NewHistogram(`requests_duration_seconds{route="` + routeName + `"}`)

	return func(w http.ResponseWriter, r *http.Request, p P) error {
		startTime := time.Now()
		defer requestDuration.UpdateDuration(startTime)

		err := h.Serve(w, r, p)

		return err
	}
}

func (app *app) initDatabaseMetrics() {
	databaseSizeBytes = metrics.NewGauge(`database_size_bytes{database="frontpage"}`, func() float64 {
		size, _, _, err := app.ndb.getDatabaseStats()
		if err != nil {
			app.logger.Error("getDatabaseStats", err)
			return 0
		}
		return float64(size)
	})

	databaseFragmentationPercent = metrics.NewGauge(`database_fragmentation_percent{database="frontpage"}`, func() float64 {
		_, _, fragmentation, err := app.ndb.getDatabaseStats()
		if err != nil {
			app.logger.Error("getDatabaseStats", err)
			return 0
		}
		return fragmentation
	})
}
