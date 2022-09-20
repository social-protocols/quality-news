package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/johnwarden/hn"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

func main() {
	fmt.Println("In main")

	sqliteDataDir := os.Getenv("SQLITE_DATA_DIR")
	if sqliteDataDir == "" {
		panic("SQLITE_DATA_DIR not set")
	}

	db, err := openNewsDatabase(sqliteDataDir)

	if err != nil {
		log.Fatal(err)
	}

	defer db.close()

	logger := newLogger(logLevelInfo)

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 5 * time.Second

	retryClient.Logger = logger

	c := hn.NewClient(retryClient.StandardClient())

	go rankCrawler(db, c, logger)

	httpServer(db)

}

func httpServer(db newsDatabase) {

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	http.HandleFunc("/", frontpageHandler(db))

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
