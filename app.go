package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

//go:embed templates/*
//go:embed sql/*
var resources embed.FS

type app struct {
	ndb            newsDatabase
	httpClient     *http.Client
	logger         leveledLogger
	cacheSize int
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

	httpClient := retryClient.StandardClient()

	return app{
		httpClient:     httpClient,
		logger:         logger,
		ndb:            db,
		cacheSize: cacheSize,
	}
}

func (app app) cleanup() {
	app.ndb.close()
}
