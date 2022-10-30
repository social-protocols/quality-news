package main

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"

	// "github.com/dyninc/qstring"
	"github.com/gorilla/schema"

	"github.com/pkg/errors"

	"github.com/julienschmidt/httprouter"
)

//go:embed static
var staticFS embed.FS

func (app app) httpServer() {

	l := app.logger

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	staticRoot, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	router := httprouter.New()
	router.ServeFiles("/static/*filepath", http.FS(staticRoot))
	router.GET("/", app.frontpageHandler("quality"))
	router.GET("/hntop", app.frontpageHandler("hntop"))
	router.GET("/stats/:storyID", app.statsHandler())
	router.GET("/stats/:storyID/ranks.png", app.plotHandler(ranksPlot))
	router.GET("/stats/:storyID/upvotes.png", app.plotHandler(upvotesPlot))
	router.GET("/stats/:storyID/upvoterate.png", app.plotHandler(upvoteRatePlot))

	l.Info("HTTP server listening", "port", port)
	l.Fatal(http.ListenAndServe(":"+port, router))
}

var decoder = schema.NewDecoder()

func (app app) frontpageHandler(ranking string) func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	logger := app.logger

	return middleware(logger, func(w http.ResponseWriter, r *http.Request, params FrontPageParams) error {

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
			b, _, err = app.generateFrontPage(ranking, params)
			if err != nil {
				return errors.Wrap(err, "renderFrontPage")
			}
		} else {
			b = app.generatedPages[ranking]
		}

		_, err = w.Write(b)

		if err != nil {
			return errors.Wrap(err, "write response")
		}
		return nil

	})
}

func (app app) statsHandler() httprouter.Handle {

	return middleware(app.logger, func(w http.ResponseWriter, r *http.Request, params StatsPageParams) error {

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Encoding", "gzip")

		var b bytes.Buffer

		zw := gzip.NewWriter(&b)
		defer zw.Close()

		err := statsPage(zw, r, params)
		if err != nil {
			return err
		}

		zw.Close()
		_, err = w.Write(b.Bytes())
		return err

	})

}

func (app app) plotHandler(plotWriter func(ndb newsDatabase, storyID int) (io.WriterTo, error)) httprouter.Handle {
	return middleware(app.logger, func(w http.ResponseWriter, r *http.Request, params StatsPageParams) error {
		writerTo, err := plotWriter(app.ndb, params.StoryID)
		if err != nil {
			return err
		}

		w.Header().Set("Content-Type", "image/png")
		_, err = writerTo.WriteTo(w)
		return err
	})
}
