package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	// "github.com/dyninc/qstring"

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

	listenAddress := os.Getenv("LISTEN_ADDRESS")

	staticRoot, err := fs.Sub(staticFS, "static")
	if err != nil {
		LogFatal(l, "fs.Sub", err)
	}

	server := &http.Server{
		Addr:              listenAddress + ":" + port,
		WriteTimeout:      writeTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	router := httprouter.New()
	router.GET("/static/*filepath", app.serveFiles(http.FS(staticRoot)))

	router.GET("/", middleware("hntop", l, onPanic, app.frontpageHandler("hntop")))
	// router.GET("/hntop", middleware("hntop", l, onPanic, app.frontpageHandler("hntop")))
	router.GET("/new", middleware("new", l, onPanic, app.frontpageHandler("new")))
	router.GET("/best", middleware("best", l, onPanic, app.frontpageHandler("best")))
	router.GET("/ask", middleware("ask", l, onPanic, app.frontpageHandler("ask")))
	router.GET("/show", middleware("show", l, onPanic, app.frontpageHandler("show")))
	router.GET("/raw", middleware("raw", l, onPanic, app.frontpageHandler("raw")))
	router.GET("/highdelta", middleware("highdelta", l, onPanic, app.frontpageHandler("highdelta")))
	router.GET("/lowdelta", middleware("lowdelta", l, onPanic, app.frontpageHandler("lowdelta")))
	router.GET("/stats", middleware("stats", l, onPanic, app.statsHandler()))
	router.GET("/about", middleware("about", l, onPanic, app.aboutHandler()))

	router.GET("/test-redirect", middleware("test-redirect", l, onPanic, app.testRedirectHandler()))

	server.Handler = app.cacheAndCompressMiddleware(router)

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
		if params.PastTime == 0 {
			params.PastTime = defaultFrontPageParams.PastTime
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

func (app app) serveFiles(root http.FileSystem) func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	fileServer := http.FileServer(root)

	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		w.Header().Set("Cache-Control", "public, max-age=86400") // 1 hours
		r.URL.Path = p.ByName("filepath")
		fileServer.ServeHTTP(w, r)
	}
}

var nonCanonicalDomains = map[string]string{
	"social-protocols-news.fly.dev:443": "news.social-protocols.org",
	"127.0.0.1:8080":                    "localhost:8080", // just for testing
}

var canonicalDomains = getValues(nonCanonicalDomains)

func (app app) testRedirectHandler() func(http.ResponseWriter, *http.Request, struct{}) error {
	logger := app.logger
	return func(w http.ResponseWriter, r *http.Request, _ struct{}) error {
		// Redirect any non-canonical domain to the corresponding canonical domain.
		for nonCanonicalDomain, canonicalDomain := range nonCanonicalDomains {
			if r.Host == nonCanonicalDomain {
				url := "https://" + canonicalDomain + r.RequestURI
				logger.Info("Redirecting to", "url", url, "request_host", r.Host)
				http.Redirect(w, r, url, 301)
				return nil
			}
		}
		isCanonical := false
		for _, canonicalDomain := range canonicalDomains {
			if strings.HasPrefix(r.Host, canonicalDomain) {
				isCanonical = true
				break
			}
		}
		if !isCanonical {
			return fmt.Errorf("Invalid request host: %s", r.Host)
		}

		_, _ = w.Write([]byte("Hello, World!\n"))
		return nil
	}
}
