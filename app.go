package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/johnwarden/hn"
	"github.com/julienschmidt/httprouter"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

func main() {
	fmt.Println("In main")

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

	go rankCrawler(db, c, logger)

	httpServer(db, logger)

}

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

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
