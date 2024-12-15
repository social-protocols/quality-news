//nolint:typecheck
package main

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

const (
	// writeTimeout      = 2500 * time.Millisecond
	writeTimeout      = 60 * time.Second
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
		WriteTimeout:      writeTimeout - 100*time.Millisecond,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	router := httprouter.New()
	router.GET("/static/*filepath", app.serveFiles(http.FS(staticRoot)))

	router.GET("/", middleware("hntop", l, onPanic, app.frontpageHandler("hntop")))
	router.GET("/new", middleware("new", l, onPanic, app.frontpageHandler("new")))
	router.GET("/top", middleware("top", l, onPanic, app.frontpageHandler("hntop")))
	router.GET("/best", middleware("best", l, onPanic, app.frontpageHandler("best")))
	router.GET("/ask", middleware("ask", l, onPanic, app.frontpageHandler("ask")))
	router.GET("/show", middleware("show", l, onPanic, app.frontpageHandler("show")))
	router.GET("/raw", middleware("raw", l, onPanic, app.frontpageHandler("raw")))
	router.GET("/fair", middleware("fair", l, onPanic, app.frontpageHandler("fair")))
	router.GET("/upvoterate", middleware("upvoterate", l, onPanic, app.frontpageHandler("upvoterate")))
	router.GET("/best-upvoterate", middleware("best-upvoterate", l, onPanic, app.frontpageHandler("best-upvoterate")))
	router.GET("/penalties", middleware("penalties", l, onPanic, app.frontpageHandler("penalties")))
	router.GET("/boosts", middleware("boosts", l, onPanic, app.frontpageHandler("boosts")))
	router.GET("/resubmissions", middleware("resubmissions", l, onPanic, app.frontpageHandler("resubmissions")))
	router.GET("/stats", middleware("stats", l, onPanic, app.statsHandler()))
	router.GET("/about", middleware("about", l, onPanic, app.aboutHandler()))
	router.GET("/algorithms", middleware("algorithms", l, onPanic, app.algorithmsHandler()))

	router.POST("/vote", middleware("upvote", l, onPanic, app.voteHandler()))

	router.GET("/score", middleware("score", l, onPanic, app.scoreHandler()))

	router.GET("/login", middleware("login", l, onPanic, app.loginHandler()))
	router.GET("/logout", middleware("logout", l, onPanic, app.logoutHandler()))

	router.GET("/health", middleware("health", l, onPanic, app.healthHandler()))
	router.GET("/crawl-health", middleware("crawl-health", l, onPanic, app.crawlHealthHandler()))

	server.Handler = app.preRouterMiddleware(router, writeTimeout-100*time.Millisecond)

	return server
}

func (app app) frontpageHandler(ranking string) func(http.ResponseWriter, *http.Request, OptionalFrontPageParams) error {
	return func(w http.ResponseWriter, r *http.Request, params OptionalFrontPageParams) error {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		err := app.serveFrontPage(r, w, ranking, params.WithDefaults())
		return errors.Wrap(err, "serveFrontPage")
	}
}

func (app app) statsHandler() func(http.ResponseWriter, *http.Request, StatsPageParams) error {
	return func(w http.ResponseWriter, r *http.Request, params StatsPageParams) error {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		userID := app.getUserID(r)
		return app.statsPage(w, r, params, userID)
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
