package main

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/johnwarden/hn"
	"golang.org/x/exp/slog"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

type app struct {
	ndb                newsDatabase
	hnClient           *hn.Client
	httpClient         *http.Client
	logger             *slog.Logger
	cacheSize          int
	archiveTriggerChan chan context.Context
}

func initApp() app {
	var err error
	var cacheSize int
	{
		s := os.Getenv("CACHE_SIZE")
		if s != "" {
			cacheSize, err = strconv.Atoi(s)
			if err != nil {
				LogFatal(slog.Default(), "CACHE_SIZE", err)
			}
		}
	}

	logLevelString := os.Getenv("LOG_LEVEL")
	logFormatString := os.Getenv("LOG_FORMAT")
	logger := newLogger(logLevelString, logFormatString)

	logger.Info("Initializing application")

	sqliteDataDir := os.Getenv("SQLITE_DATA_DIR")
	if sqliteDataDir == "" {
		panic("SQLITE_DATA_DIR not set")
	}

	logger.Info("Opening database", "dataDir", sqliteDataDir)
	db, err := openNewsDatabase(sqliteDataDir, logger)
	if err != nil {
		LogFatal(logger, "openNewsDatabase", err)
	}
	logger.Info("Database opened successfully")

	logger.Info("Initializing HTTP client")
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 5 * time.Second

	retryClient.Logger = wrapLoggerForRetryableHTTPClient(logger)

	httpClient := retryClient.StandardClient()

	hnClient := hn.NewClient(httpClient)

	logger.Info("Application initialization complete")

	return app{
		httpClient:         httpClient,
		hnClient:           hnClient,
		logger:             logger,
		ndb:                db,
		cacheSize:          cacheSize,
		archiveTriggerChan: make(chan context.Context, 1), // Buffer size 1: one signal can queue while processing
	}
}

func (app app) cleanup() {
	app.ndb.close()
}
