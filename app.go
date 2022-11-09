package main

import (
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

type app struct {
	ndb              newsDatabase
	httpClient       *http.Client
	logger           leveledLogger
	generatedPages   map[string][]byte
	generatedPagesMU *sync.Mutex
}

func initApp() app {
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

	var generatedPagesMU sync.Mutex
	return app{
		httpClient:       httpClient,
		logger:           logger,
		ndb:              db,
		generatedPages:   make(map[string][]byte),
		generatedPagesMU: &generatedPagesMU,
	}
}

func (app app) cleanup() {
	app.ndb.close()
}
