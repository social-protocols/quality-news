package main

import (
	"context"
	"embed"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

//go:embed templates/*
//go:embed sql/*
var resources embed.FS

type app struct {
	ndb        newsDatabase
	httpClient *http.Client
	logger     leveledLogger
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
	proxyURL, err := url.Parse("http://localhost:8081")
	if err != nil {
		panic(err)
	}
	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		retryClient.HTTPClient.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}
		if resp.StatusCode >= 500 {
			crawlRequestErrors.Inc()
			return true, nil
		}
		return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	}

	// add proxy support

	retryClient.Logger = wrapLoggerForRetryableHTTPClient(logger)

	httpClient := retryClient.StandardClient()

	return app{
		httpClient: httpClient,
		logger:     logger,
		ndb:        db,
		cacheSize:  cacheSize,
	}
}

func (app app) cleanup() {
	app.ndb.close()
}
