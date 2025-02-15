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
	archiveErrorsTotal          = metrics.NewCounter(`errors_total{type="archive"}`)
	requestErrorsTotal          = metrics.NewCounter(`errors_total{type="request"}`)
	crawlDuration               = metrics.NewHistogram("crawl_duration_seconds")
	crawlPostprocessingDuration = metrics.NewHistogram("crawl_postprocessing_duration_seconds")
	archivingAndPurgeDuration   = metrics.NewHistogram("archiving_and_purge_duration_seconds")

	upvotesTotal         = metrics.NewCounter(`upvotes_total`)
	submissionsTotal     = metrics.NewCounter(`submissions_total`)
	storiesArchivedTotal = metrics.NewCounter(`stories_archived_total`)

	databaseSizeBytes            *metrics.Gauge
	databaseFragmentationPercent *metrics.Gauge
	vacuumOperationsTotal        = metrics.NewCounter(`database_vacuum_operations_total{database="frontpage"}`)

	// Store histograms per route to avoid duplicate registration
	routeHistograms = make(map[string]*metrics.Histogram)
)

// getRouteHistogram returns an existing histogram for a route or creates a new one
func getRouteHistogram(routeName string) *metrics.Histogram {
	if h, exists := routeHistograms[routeName]; exists {
		return h
	}
	h := metrics.NewHistogram(`requests_duration_seconds{route="` + routeName + `"}`)
	routeHistograms[routeName] = h
	return h
}

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
	requestDuration := getRouteHistogram(routeName)

	return func(w http.ResponseWriter, r *http.Request, p P) error {
		var startTime time.Time
		if r.Method != http.MethodHead {
			startTime = time.Now()
		}

		err := h.Serve(w, r, p)

		if r.Method != http.MethodHead && routeName != "health" && routeName != "crawl-health" {
			requestDuration.UpdateDuration(startTime)
		}

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
