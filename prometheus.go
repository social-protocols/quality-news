package main

import (
	"context"
	"net/http"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/johnwarden/httperror"
	"golang.org/x/exp/slog"
)

// Register various metrics.
// Metric name may contain labels in Prometheus format - see below.

var (
	crawlErrorsTotal            = metrics.NewCounter(`errors_total{type="crawl"}`)
	crawlRequestErrors          = metrics.NewCounter(`request_errors{type="crawl"}`)
	requestErrorsTotal          = metrics.NewCounter(`errors_total{type="request"}`)
	crawlDuration               = metrics.NewHistogram("crawl_duration_seconds")
	crawlPostprocessingDuration = metrics.NewHistogram("crawl_postprocessing_duration_seconds")

	upvotesTotal     = metrics.NewCounter(`upvotes_total`)
	submissionsTotal = metrics.NewCounter(`submissions_total`)
)

var generateFrontpageMetrics map[string]*metrics.Histogram

func init() {
	generateFrontpageMetrics = make(map[string]*metrics.Histogram)
	for _, ranking := range []string{"hntop", "quality", "new"} {
		generateFrontpageMetrics[ranking] = metrics.NewHistogram(`generate_frontpage_duration_seconds{ranking="` + ranking + `"}`)
	}
}

func servePrometheusMetrics() func(ctx context.Context) error {
	mux := http.NewServeMux()

	// Export all the registered metrics in Prometheus format at `/metrics` http path.
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.WritePrometheus(w, true)
	})

	s := &http.Server{
		Addr:    ":9091",
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

		err := h.Serve(w, r, p)

		requestDuration.UpdateDuration(startTime)

		return err
	}
}
