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
	shutdownPrometheusServer := servePrometheusMetrics()

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

	shutdown := func() {
		// cancel the current background context
		cancelContext()

		shutdownPrometheusServer(ctx)

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
		logger.Info("Main loop exited. Terminating process")

		os.Exit(0)
	}()

	err = app.generateAndCacheFrontPages(ctx)
	if err != nil {
		logger.Fatal(err)
	}

	httpServer = app.httpServer(
		func(error) {
			logger.Info("Panic in HTTP handler. Shutting down")
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

	app.mainLoop(ctx)
}

type app struct {
	ndb              newsDatabase
	hnClient         *hn.Client
	logger           leveledLogger
	generatedPages   map[string][]byte
	generatedPagesMU *sync.Mutex
}

// sleeps but stops sleeping if the context is cancelled.
func sleepCtx(ctx context.Context, dur time.Duration) {
	ch := make(chan bool)

	go func() {
		<-time.After(dur)
		ch <- true
	}()

	select {
	case <-ch:
		break
	case <-ctx.Done():
		break
	}
}

func (app app) mainLoop(ctx context.Context) {
	logger := app.logger

	lastCrawlTime, err := app.ndb.selectLastCrawlTime()
	if err != nil {
		logger.Err(errors.Wrap(err, "selectLastCrawlTime"))
		os.Exit(2)
	}

	t := time.Now().Unix()

	elapsed := int(t) - lastCrawlTime

	// If it has been more than a minute since our last crawl,
	// then crawl right away.
	if elapsed > 60 {
		logger.Info("More than 60 seconds since last crawl. Crawling now.")
		if err = app.crawlAndGenerate(ctx); err != nil {
			logger.Err(err)

			if errors.Is(err, context.Canceled) {
				return
			}
		}
	} else {
		logger.Info("Less than 60 seconds since last crawl. Waiting.", "seconds", 60-time.Now().Unix()%60)
	}

	// And now set a ticker so we crawl every minute going forward
	ticker := make(chan struct{})

	// Now the next crawl happens on the minute. Make the first tick happen at the next
	// Minute mark.
	go func() {
		t = time.Now().Unix()
		logger.Debug("Waiting for next minute mark", "seconds", time.Duration(60-t%60), "now", time.Now())
		<-time.After(time.Duration(60-t%60) * time.Second)
		ticker <- struct{}{}
	}()

	for {
		select {
		case <-ticker:
			logger.Debug("Got ticker", "now", time.Now())

			// Set the next tick at the minute mark. We use this instead of using
			// time.NewTicker because in dev mode our app can be suspended, and I
			// want to see all the timestamps in the DB as multiples of 60.
			go func() {
				t := time.Now().Unix()
				<-time.After(time.Duration(60-t%60) * time.Second)
				ticker <- struct{}{}
			}()
			if err = app.crawlAndGenerate(ctx); err != nil {
				logger.Err(err)
			}

		case <-ctx.Done():
			return
		}
	}
}

func (app app) crawlAndGenerate(ctx context.Context) (err error) {
	if err = app.crawlHN(ctx); err != nil {
		crawlErrorsTotal.Inc()
		err = errors.Wrap(err, "crawlHN")
		return
	}

	if err = app.generateAndCacheFrontPages(ctx); err != nil {
		generateFrontpageErrorsTotal.Inc()
		err = errors.Wrap(err, "renderFrontPages")
		return
	}

	return
}
