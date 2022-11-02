package main

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	// "github.com/dyninc/qstring"
	"github.com/gorilla/schema"

	"github.com/pkg/errors"

	"github.com/julienschmidt/httprouter"
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
	// router.GET("/offtopic", middleware(l, onPanic, app.frontpageHandler("offtopic")))
	router.GET("/stats/:storyID", middleware("stats", l, onPanic, app.statsHandler()))
	router.GET("/stats/:storyID/ranks.png", middleware("ranks-plot", l, onPanic, app.plotHandler(ranksPlot)))
	router.GET("/stats/:storyID/upvotes.png", middleware("upvotes-plot", l, onPanic, app.plotHandler(upvotesPlot)))
	router.GET("/stats/:storyID/upvoterate.png", middleware("upvoterate-plot", l, onPanic, app.plotHandler(upvoteRatePlot)))

	router.GET("/altcharts/go-chart", middleware(l, onPanic, app.altChartsGoChart()))
	router.GET("/altcharts/go-echarts", middleware(l, onPanic, app.altChartsGoEcharts()))

	server.Handler = router

	return server
}

var decoder = schema.NewDecoder()

func (app app) frontpageHandler(ranking string) func(http.ResponseWriter, *http.Request, FrontPageParams) error {
	logger := app.logger

	return func(w http.ResponseWriter, r *http.Request, params FrontPageParams) error {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Encoding", "gzip")

		var b []byte
		var err error
		if params != noFrontPageParams {

			if params.Gravity == 0 {
				params.Gravity = defaultFrontPageParams.Gravity
			}
			if params.OverallPriorWeight == 0 {
				params.OverallPriorWeight = defaultFrontPageParams.OverallPriorWeight
			}
			if params.PriorWeight == 0 {
				params.PriorWeight = defaultFrontPageParams.PriorWeight
			}

			logger.Info("Generating front page with custom parameters", "params", params)
			b, _, err = app.generateFrontPage(r.Context(), ranking, params)
			if err != nil {
				return errors.Wrap(err, "renderFrontPage")
			}
		} else {
			b = app.generatedPages[ranking]
		}

		if len(b) == 0 {
			return fmt.Errorf("Front page has not been generated")
		}

		_, err = w.Write(b)

		if err != nil {
			return errors.Wrap(err, "write response")
		}
		return nil
	}
}

func (app app) statsHandler() func(http.ResponseWriter, *http.Request, StatsPageParams) error {
	return func(w http.ResponseWriter, r *http.Request, params StatsPageParams) error {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Encoding", "gzip")

		var b bytes.Buffer

		zw := gzip.NewWriter(&b)
		defer zw.Close()

		err := statsPage(app.ndb, zw, r, params)
		if err != nil {
			return err
		}

		zw.Close()
		_, err = w.Write(b.Bytes())
		return err
	}
}

func (app app) plotHandler(plotWriter func(ndb newsDatabase, storyID int) (io.WriterTo, error)) func(http.ResponseWriter, *http.Request, StatsPageParams) error {
	return func(w http.ResponseWriter, r *http.Request, params StatsPageParams) error {
		writerTo, err := plotWriter(app.ndb, params.StoryID)
		if err != nil {
			return err
		}

		w.Header().Set("Content-Type", "image/png")
		_, err = writerTo.WriteTo(w)
		return err
	}
}
