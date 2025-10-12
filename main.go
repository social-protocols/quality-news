package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

const maxShutDownTimeout = 5 * time.Second

func runRecoveryMode() {
	logger := newLogger(os.Getenv("LOG_LEVEL"), os.Getenv("LOG_FORMAT"))

	logger.Info("===========================================")
	logger.Info("RECOVERY MODE ACTIVE")
	logger.Info("Database access disabled - safe for manual restoration")
	logger.Info("===========================================")
	logger.Info("To restore database:")
	logger.Info("1. fly ssh console")
	logger.Info("2. cd /data")
	logger.Info("3. rm frontpage.sqlite frontpage.sqlite-wal frontpage.sqlite-shm")
	logger.Info("4. cp frontpage_backup_YYYY_MM_DD.sqlite frontpage.sqlite")
	logger.Info("5. exit")
	logger.Info("6. fly secrets unset RECOVERY_MODE")
	logger.Info("7. App will auto-restart in normal mode")
	logger.Info("===========================================")

	// Simple HTTP server for health checks
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("RECOVERY MODE - Database restoration in progress\n"))
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	logger.Info("HTTP server listening in recovery mode", "address", ":8080")

	// Block forever (or until killed)
	if err := server.ListenAndServe(); err != nil {
		logger.Error("Recovery mode server error", err)
	}
}

func main() {
	// Check for recovery mode - minimal app for database restoration
	if os.Getenv("RECOVERY_MODE") == "true" {
		runRecoveryMode()
		return
	}

	app := initApp()
	defer app.cleanup()

	logger := app.logger

	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	shutdownPrometheusServer := servePrometheusMetrics()

	// Listen for a soft kill signal (INT, TERM, HUP)
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// shutdown function call in case of 1) panic 2) soft kill signal
	var httpServer *http.Server // this variable included in shutdown closure

	shutdown := func() {
		// cancel the current background context
		cancelContext()

		err := shutdownPrometheusServer(ctx)
		if err != nil {
			logger.Error("shutdownPrometheusServer", err)
		}

		if httpServer != nil {
			logger.Info("Shutting down HTTP server")
			// shut down the HTTP server with a timeout in case the server doesn't want to shut down.
			// use background context, because we just cancelled ctx
			ctxWithTimeout, cancel := context.WithTimeout(context.Background(), maxShutDownTimeout)
			defer cancel()
			err := httpServer.Shutdown(ctxWithTimeout)
			if err != nil {
				logger.Error("httpServer.Shutdown", err)
				// if server doesn't respond to shutdown signal, nothing remains but to panic.
				panic("HTTP server shutdown failed")
			}

			logger.Info("HTTP server shutdown complete")
		}
	}

	go func() {
		sig := <-c

		// Clean shutdown
		logger.Info("Received shutdown signal", "signal", sig)
		shutdown()

		// now exit process
		logger.Info("Main loop exited. Terminating process")

		os.Exit(0)
	}()

	httpServer = app.httpServer(
		func(error) {
			logger.Info("Panic in HTTP handler. Shutting down")
			shutdown()
			os.Exit(2)
		},
	)

	go func() {
		logger.Info("HTTP server listening", "address", httpServer.Addr)
		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server.ListenAndServe", err)
		}
		logger.Info("Server shut down")
	}()

	// Check if VACUUM is pending from previous weekly run OR if fragmentation is high
	// This runs BEFORE starting workers to avoid wasted work during VACUUM
	logger.Info("Checking for pending VACUUM operation or high fragmentation")

	// One-time check: if fragmentation > 15%, trigger VACUUM immediately on this startup
	_, _, fragmentation, err := app.ndb.getDatabaseStats()
	if err == nil && fragmentation > 15.0 {
		markerPath := fmt.Sprintf("%s/.vacuum_pending", app.ndb.sqliteDataDir)
		if _, statErr := os.Stat(markerPath); os.IsNotExist(statErr) {
			logger.Info("High fragmentation detected - triggering immediate VACUUM",
				"fragmentation_pct", fragmentation)
			if markerFile, createErr := os.Create(markerPath); createErr == nil {
				markerFile.Close()
				logger.Info("VACUUM marker created - will execute before starting workers")
			}
		}
	}

	if err := performVacuumOnStartup(app.ndb, logger); err != nil {
		logger.Error("Startup VACUUM failed", err)
		logger.Warn("Will retry next Sunday during scheduled maintenance")
		// Continue starting workers even if VACUUM fails
	}

	// Start the archive worker (runs every 5 minutes)
	go app.archiveWorker(ctx)

	// Start the purge worker (runs during idle time between crawls)
	go app.purgeWorker(ctx)

	// Start the vacuum worker (runs Sunday early morning)
	go app.vacuumWorker(ctx)

	app.mainLoop(ctx)
}

func (app app) mainLoop(ctx context.Context) {
	logger := app.logger

	lastCrawlTime, err := app.ndb.selectLastCrawlTime()
	if err != nil {
		LogFatal(logger, "selectLastCrawlTime", err)
	}

	t := time.Now().Unix()

	elapsed := int(t) - lastCrawlTime

	// If it has been more than a minute since our last crawl,
	// then crawl right away.
	if elapsed >= 60 {
		logger.Info("60 seconds since last crawl. Crawling now.")
		if err = app.crawlAndPostprocess(ctx); err != nil {
			logger.Error("crawlAndPostprocess", err)

			if errors.Is(err, context.Canceled) {
				return
			}
		}
	} else {
		logger.Info("Less than 60 seconds since last crawl.", "waitSeconds", 60-time.Now().Unix()%60)
	}

	// And now set a ticker so we crawl every minute going forward
	ticker := make(chan int64)

	// Make the first tick happen at the next
	// Minute mark.
	go func() {
		t := time.Now().Unix()
		delay := 60 - t%60
		<-time.After(time.Duration(delay) * time.Second)
		ticker <- t + delay
	}()

	for {
		select {
		case <-ticker:
			t := time.Now().Unix()
			// Set the next tick at the minute mark. We use this instead of using
			// time.NewTicker because in dev mode our app can be suspended, and I
			// want to see all the timestamps in the DB as multiples of 60.
			delay := 60 - t%60
			nextTickTime := t + delay
			go func() {
				<-time.After(time.Duration(delay) * time.Second)
				ticker <- nextTickTime
			}()

			logger.Info("Beginning crawl")

			// Create a context with deadline for both crawl and idle period
			crawlCtx, cancel := context.WithDeadline(ctx, time.Unix(nextTickTime-1, 0))
			defer cancel()

			if err = app.crawlAndPostprocess(crawlCtx); err != nil {
				logger.Error("crawlAndPostprocess", err)
			} else {
				app.logger.Info("Finished crawl and postprocess")

				// Only send idle context if we have enough time (at least 5 seconds)
				if delay >= 5 {
					// Try to send the context to the purge worker (non-blocking)
					select {
					case app.archiveTriggerChan <- crawlCtx:
						app.logger.Debug("Sent idle context to purge worker", "available_seconds", delay)
					default:
						app.logger.Warn("Purge trigger channel full, signal dropped - purge worker may be backed up")
					}
				} else {
					app.logger.Debug("Skipping idle context - not enough time", "delay", delay)
				}
			}

		case <-ctx.Done():
			return
		}
	}
}
