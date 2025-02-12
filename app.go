package main

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/johnwarden/hn"
	"golang.org/x/exp/slog"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

type app struct {
	ndb        newsDatabase
	hnClient   *hn.Client
	httpClient *http.Client
	logger     *slog.Logger
	cacheSize  int
}

func initApp() app {
	var err error
	var cacheSize int
	{
		s := os.Getenv("CACHE_SIZE")
		if s != "" {
			cacheSize, err = strconv.Atoi(s)
			if err != nil {
				panic("Couldn't parse CACHE_SIZE")
			}
		}
	}

	logLevelString := os.Getenv("LOG_LEVEL")
	logFormatString := os.Getenv("LOG_FORMAT")
	logger := newLogger(logLevelString, logFormatString)

	sqliteDataDir := os.Getenv("SQLITE_DATA_DIR")
	if sqliteDataDir == "" {
		panic("SQLITE_DATA_DIR not set")
	}

	db, err := openNewsDatabase(sqliteDataDir)
	if err != nil {
		LogFatal(logger, "openNewsDatabase", err)
	}

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 5 * time.Second

	retryClient.Logger = wrapLoggerForRetryableHTTPClient(logger)

	httpClient := retryClient.StandardClient()

	hnClient := hn.NewClient(httpClient)

	return app{
		httpClient: httpClient,
		hnClient:   hnClient,
		logger:     logger,
		ndb:        db,
		cacheSize:  cacheSize,
	}
}

func (app app) cleanup() {
	app.ndb.close()
}
