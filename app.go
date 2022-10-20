package main

import (
	"log"
	"os"
	"time"
	
	"github.com/pkg/errors"
    "github.com/johnwarden/hn"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
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

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 5 * time.Second

	{
		l := logger
		l.level = logLevelInfo
		retryClient.Logger = l // ignore debug messages from this retry client.
	}

	err = renderFrontPages(db, logger)
	if err != nil {
		logger.Fatal(err)
	}

	c := hn.NewClient(retryClient.StandardClient())

	go mainLoop(db, c, logger)

	httpServer(db, logger)

}


func mainLoop(ndb newsDatabase, client *hn.Client, logger leveledLogger) {

	err := crawlAndRender(ndb, client, logger)
	if err != nil {
		logger.Err(err)
	}

	ticker := time.NewTicker(60 * time.Second)
	quit := make(chan struct{})

	for {
		select {
		case <-ticker.C:
			err := crawlAndRender(ndb, client, logger)
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


func crawlAndRender(ndb newsDatabase, client *hn.Client, logger leveledLogger) error {
	err := crawlHN(ndb, client, logger)
	if err != nil {
		return errors.Wrap(err, "crawlHN")
	}
	logger.Debug("Now render front pages")
	err = renderFrontPages(ndb, logger)
	if err != nil {
		return errors.Wrap(err, "renderFrontPages")
	}
	return nil
}
