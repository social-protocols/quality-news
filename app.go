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

const maxShutDownTimeout = 90 * time.Second

func main() {
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

	app := app{
		hnClient:       hnClient,
		logger:         logger,
		ndb:            db,
		generatedPages: make(map[string][]byte),
	}

	// Listen for a soft kill signal (INT, TERM, HUP)
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)


	err = app.generateAndCacheFrontPages()
	if err != nil {
		logger.Fatal(err)
	}

	app.mainLoop()

	// shutdown function call in case of 1) panic 2) soft kill signal
	var httpServer *http.Server // this variable included in shutdown closure
	shutdown := func() {

		// shut down the HTTP server with a timeout in case the server doesn't want to shut down.
		ctx := context.Background()
		ctxWithTimeout, cancel := context.WithTimeout(ctx, maxShutDownTimeout)
		defer cancel()
		err := httpServer.Shutdown(ctxWithTimeout)
		if err != nil {
			logger.Err(errors.Wrap(err, "httpServer.Shutdown"))
			// if server doesn't respond to shutdown signal, nothing remains but to panic.
			panic("HTTP server shutdown failed")
		}

		logger.Info("HTTP server shutdown complete")

		// now exit process
		logger.Info("Main loop exited. Terminating process.")
	}

	httpServer = app.httpServer(
		func() {
			logger.Info("Panic in HTTP handler. Shutting down.")
			shutdown()
			os.Exit(2)
		},
	)

	logger.Info("Waiting for shutdown signal")

	sig := <-c

	// Clean shutdown
	logger.Info("Received shutdown signal", "signal", sig)
	shutdown()
	os.Exit(0)
}

type app struct {
	ndb            newsDatabase
	hnClient       *hn.Client
	logger         leveledLogger
	generatedPages map[string][]byte
}

func (app app) mainLoop() {

	logger := app.logger

	err := app.crawlAndGenerate()
	if err != nil {
		logger.Err(err)
	}

	ticker := time.NewTicker(60 * time.Second)
	quit := make(chan struct{})

	for {
		select {
		case <-ticker.C:
			err := app.crawlAndGenerate()
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

func (app app) crawlAndGenerate() error {

	err := app.crawlHN()
	if err != nil {
		return errors.Wrap(err, "crawlHN")
	}

	err = app.generateAndCacheFrontPages()
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
