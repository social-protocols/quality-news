package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	// "github.com/dyninc/qstring"

	"github.com/NYTimes/gziphandler"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

const (
	writeTimeout      = 2500 * time.Millisecond
	readHeaderTimeout = 5 * time.Second
)

//go:embed static
var staticFS embed.FS

func (app app) httpServer(onPanic func(error)) *http.Server {
	l := app.logger

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	staticRoot, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:              ":" + port,
		WriteTimeout:      writeTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	router := httprouter.New()
	router.ServeFiles("/static/*filepath", http.FS(staticRoot))
	router.GET("/", middleware("qntop", l, onPanic, app.frontpageHandler("quality")))
	router.GET("/hntop", middleware("hntop", l, onPanic, app.frontpageHandler("hntop")))
	router.GET("/stats", middleware("stats", l, onPanic, app.statsHandler()))

	router.GET("/plots/ranks.json", middleware("ranks-plotdata", l, onPanic, app.ranksDataJSON()))
	router.GET("/plots/upvotes.json", middleware("upvotes-plotdata", l, onPanic, app.upvotesDataJSON()))
	router.GET("/plots/upvoterate.json", middleware("upvoterate-plotdata", l, onPanic, app.upvoteRateDataJSON()))

	// We could improve our cache+compress middleware setup. We cache before
	// we compress, because the cache middleware we use here doesn't
	// recognize the accept-encoding header, and if we compressed before we
	// cache, cache entries would be randomly compressed or not, regardless
	// of the accept-encoding header. The downside is that requests are
	// cached uncompressed. A compressed-cache middleware would be nice. Also
	// our cache-control headers should be synced with the exact cache
	// expiration time, which should be synced with the crawl. But what we
	// have here is simple and good enough.
	server.Handler = gziphandler.GzipHandler(app.cacheMiddleware(router))

	return server
}

func (app app) frontpageHandler(ranking string) func(http.ResponseWriter, *http.Request, FrontPageParams) error {
	return func(w http.ResponseWriter, r *http.Request, params FrontPageParams) error {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		if params.Gravity == 0 {
			params.Gravity = defaultFrontPageParams.Gravity
		}
		if params.OverallPriorWeight == 0 {
			params.OverallPriorWeight = defaultFrontPageParams.OverallPriorWeight
		}
		if params.PriorWeight == 0 {
			params.PriorWeight = defaultFrontPageParams.PriorWeight
		}
		if params.PenaltyWeight == 0 {
			params.PenaltyWeight = defaultFrontPageParams.PenaltyWeight
		}

		err := app.serveFrontPage(r, w, ranking, params)
		return errors.Wrap(err, "serveFrontPage")
	}
}

func (app app) statsHandler() func(http.ResponseWriter, *http.Request, StatsPageParams) error {
	return func(w http.ResponseWriter, r *http.Request, params StatsPageParams) error {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		err := statsPage(app.ndb, w, r, params)
		if err != nil {
			return err
		}

		return err
	}
}
