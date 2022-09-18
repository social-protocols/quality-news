package main

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	hn "github.com/peterhellberg/hn"
)

type FrontPageData struct {
	Stories []hn.Item
}

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

func main() {
	fmt.Println("In main")

	go getNewStories()

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

func insertStory(db *sql.DB, story hn.Item) {
	log.Println("Inserting story record ...")
	insertStorySQL := `INSERT INTO stories(id, by, title, url, timestamp) VALUES (?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`
	statement, err := db.Prepare(insertStorySQL) // Prepare statement.
	// This is good to avoid SQL injections
	if err != nil {
		log.Fatalln(err.Error())
	}
	_, err = statement.Exec(story.ID, story.By, story.Title, story.URL, story.Timestamp)
	if err != nil {
		log.Fatalln(err.Error())
	}
}

func getNewStories() {

	sqliteDataDir := os.Getenv("SQLITE_DATA_DIR")
	if sqliteDataDir == "" {
		panic("SQLITE_DATA_DIR not set")
	}

	frontpageDatabaseFilename := fmt.Sprintf("%s/frontpage.sqlite", sqliteDataDir)
	fmt.Println("Database file", frontpageDatabaseFilename)
	db, err := sql.Open("sqlite3", frontpageDatabaseFilename)
	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	hn := hn.NewClient(&http.Client{
		Timeout: time.Duration(5 * time.Second),
	})

	ourMaxItem := 32891067
	theirMaxItem, err := hn.Live.MaxItem()
	if err != nil {
		log.Fatal(err)
	}

	theirMaxItem = ourMaxItem + 10

	fmt.Println("Got max item", theirMaxItem)

	//	Get the ID of the last story that has been submitted
	//

	//	Get the highest ID you have in the databse
	//
	var wg sync.WaitGroup

	for i := ourMaxItem + 1; i <= theirMaxItem; i++ {

		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			item, err := hn.Item(id)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Item type", item.Type)
			if item.Type == "story" {
				fmt.Println("Inserting story", item)
				insertStory(db, *item)
			}

		}(i)
	}

	wg.Wait()

}

func runCrawler() {
	sqliteDataDir := os.Getenv("SQLITE_DATA_DIR")

	if sqliteDataDir == "" {
		panic("SQLITE_DATA_DIR not set")
	}

	databaseFilename := fmt.Sprintf("%s/hacker-news.sqlite", sqliteDataDir)

	fmt.Println("Database file", databaseFilename)
	db, err := sql.Open("sqlite3", databaseFilename)

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
