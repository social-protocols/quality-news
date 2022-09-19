package main

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	_ "github.com/mattn/go-sqlite3"

	hn "github.com/peterhellberg/hn"
)

// FrontPageData contains the data to populate the front page template.
type FrontPageData struct {
	Stories []hn.Item
}

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

func main() {
	fmt.Println("In main")

	go storiesCrawler()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"

	}

	tmpl := template.Must(template.ParseFiles("templates/index.html.tmpl"))

	sampleStory := hn.Item{
		ID: 8863,
		By: "dhouston",
		//		Parent:      8862,
		Title:     "My YC app: Dropbox - Throw away your USB drive",
		URL:       "http://www.getdropbox.com/u/2/screencast.html",
		Timestamp: 1175714200,
	}
	sampleStories := []hn.Item{
		sampleStory,
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl.Execute(w, FrontPageData{sampleStories})
	})

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func runCrawler() {
	sqliteDataDir := os.Getenv("SQLITE_DATA_DIR")

	if sqliteDataDir == "" {
		panic("SQLITE_DATA_DIR not set")
	}

	rankDatasetDatabaseFilename := fmt.Sprintf("%s/dataset.sqlite", sqliteDataDir)

	fmt.Println("Database file", rankDatasetDatabaseFilename)
	db, err := sql.Open("sqlite3", rankDatasetDatabaseFilename)

	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	sqlQuery := "select id, gain from dataset limit 2"

	rows, err := db.Query(sqlQuery)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var gain int
		err = rows.Scan(&id, &gain)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Got Row", id, gain)
	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Successfully executed select query")
}
