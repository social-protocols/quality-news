package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

const maxShutDownTimeout = 5 * time.Second

func main() {
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

	app.mainLoop(ctx)
}

func (app app) mainLoop(ctx context.Context) {
	logger := app.logger

	// logger.Info("Archiving old stories")
	// err := app.runArchivingTasks(ctx)
	// if err != nil {
	// 	LogFatal(logger, "runArchivingTasks", err)
	// }

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
		logger.Debug("Scheduling tick at next minute mark", "seconds", delay, "now", t, "nextTick", t+delay)
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
			go func() {
				logger.Debug("Scheduling tick at next minute mark", "seconds", delay, "lastTick", t, "nextTick", t+delay)
				<-time.After(time.Duration(delay) * time.Second)
				ticker <- t + delay
			}()

			logger.Info("Beginning crawl")

			// cancel crawl if it doesn't complete 1 second before the next
			// crawl is supposed to start
			logger.Debug("Setting deadline ", "time", t, "deadline", t+delay-1)
			ctx, cancel := context.WithDeadline(ctx, time.Unix(t+delay-1, 0))
			defer cancel()

			if err = app.crawlAndPostprocess(ctx); err != nil {
				logger.Error("crawlAndPostprocess", err)
			}

			// logger.Info("Archiving old stories")
			// err := app.runArchivingTasks(ctx)
			// if err != nil {
			// 	logger.Error("runArchivingTasks", err)
			// }

			logger.Debug("Scheduling tick at next minute mark", "seconds", 60-time.Now().Unix()%60)

		case <-ctx.Done():
			return
		}
	}
}
