package main


import (
	"embed"
	"net/http"
	"io/fs"
	"os"
	"log"

	// "github.com/dyninc/qstring"
	"github.com/gorilla/schema"

	"github.com/pkg/errors"

	"github.com/julienschmidt/httprouter"


)
//go:embed static
var staticFS embed.FS

func httpServer(db newsDatabase, l leveledLogger) {

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
	router.GET("/", frontpageHandler(db, "quality", l))
	router.GET("/hntop", frontpageHandler(db, "hntop", l))

	l.Info("HTTP server listening", "port", port)
	l.Fatal(http.ListenAndServe(":"+port, router))
}


var decoder = schema.NewDecoder()

func frontpageHandler(ndb newsDatabase, ranking string, logger leveledLogger) func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	return routerHandler(logger, func(w http.ResponseWriter, r *http.Request, params FrontPageParams) error {

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

			logger.Info("Generating front page with custom parameters", "params",params)
			b, err = renderFrontPage(ndb, logger, ranking, params)
			if err != nil {
				return errors.Wrap(err, "renderFrontPage")
			}
		} else {
			b = pages[ranking]
		}

		_, err = w.Write(b)

		if err != nil {
			return errors.Wrap(err, "write response");
		}
		return nil

	})
}

