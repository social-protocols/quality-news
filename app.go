package main

import (
	"log"
	"os"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/johnwarden/hn"
	"github.com/pkg/errors"
)

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

	rankComparisonPlot(db, 33110478)

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

	err = app.generateAndCacheFrontPages()
	if err != nil {
		logger.Fatal(err)
	}

	go app.mainLoop()

	app.httpServer()

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
