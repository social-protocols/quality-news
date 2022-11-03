package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/johnwarden/hn"
	"github.com/pkg/errors"
)

const maxShutDownTimeout = 5 * time.Second

func main() {
	go servePrometheusMetrics()

	logLevelString := os.Getenv("LOG_LEVEL")

	if logLevelString == "" {
		logLevelString = "DEBUG"
	}

	sqliteDataDir := os.Getenv("SQLITE_DATA_DIR")
	if sqliteDataDir == "" {
		panic("SQLITE_DATA_DIR not set")
	}

	db, err := openNewsDatabase(sqliteDataDir)
	if err != nil {
		log.Fatal(err)
	}

	defer db.close()

	logger := newLogger(logLevelString)

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 5 * time.Second

	{
		l := logger
		l.level = logLevelInfo
		retryClient.Logger = l // ignore debug messages from this retry client.
	}

	hnClient := hn.NewClient(retryClient.StandardClient())

	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	app := app{
		hnClient:       hnClient,
		logger:         logger,
		ndb:            db,
		generatedPages: make(map[string][]byte),
	}

	// Listen for a soft kill signal (INT, TERM, HUP)
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// shutdown function call in case of 1) panic 2) soft kill signal
	var httpServer *http.Server // this variable included in shutdown closure
	quit := make(chan struct{})

	shutdown := func() {
		// cancel the current background context
		cancelContext()

		// stop the main app loop
		quit <- struct{}{}

		if httpServer != nil {
			logger.Info("Shutting down HTTP server")
			// shut down the HTTP server with a timeout in case the server doesn't want to shut down.
			// use background context, because we just cancelled ctx
			ctxWithTimeout, cancel := context.WithTimeout(context.Background(), maxShutDownTimeout)
			defer cancel()
			err := httpServer.Shutdown(ctxWithTimeout)
			if err != nil {
				logger.Err(errors.Wrap(err, "httpServer.Shutdown"))
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
		logger.Info("Main loop exited. Terminating process.")

		os.Exit(0)
	}()

	err = app.generateAndCacheFrontPages(ctx)
	if err != nil {
		logger.Fatal(err)
	}

	httpServer = app.httpServer(
		func(error) {
			logger.Info("Panic in HTTP handler. Shutting down.")
			shutdown()
			os.Exit(2)
		},
	)

	go func() {
		logger.Info("HTTP server listening", "address", httpServer.Addr)
		err = httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Err(errors.Wrap(err, "server.ListenAndServe"))
		}
		logger.Info("Server shut down")
	}()

	app.mainLoop(ctx, quit)
}

type app struct {
	ndb            newsDatabase
	hnClient       *hn.Client
	logger         leveledLogger
	generatedPages map[string][]byte
}

func (app app) mainLoop(ctx context.Context, quit chan struct{}) {
	logger := app.logger

	err := app.crawlAndGenerate(ctx)
	if err != nil {
		logger.Err(err)
	}

	ticker := time.NewTicker(60 * time.Second)

	for {
		select {
		case <-ticker.C:
			err := app.crawlAndGenerate(ctx)
			if err != nil {
				logger.Err(err)
				continue
			}

		case <-quit:
			ticker.Stop()
			return
		}
	}
}

func (app app) crawlAndGenerate(ctx context.Context) error {
	err := app.crawlHN(ctx)
	if err != nil {
		crawlErrorsTotal.Inc()
		return errors.Wrap(err, "crawlHN")
	}

	err = app.generateAndCacheFrontPages(ctx)
	if err != nil {
		return errors.Wrap(err, "renderFrontPages")
	}

	return nil
}

func (app app) insertQNRanks(ranks []int) error {
	for i, id := range ranks {
		err := app.ndb.updateQNRank(id, i+1)
		if err != nil {
			return errors.Wrap(err, "updateQNRank")
		}
	}
	return nil
}
