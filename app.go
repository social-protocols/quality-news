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
	"sync/atomic"
	"time"

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

func insertStory(db *sql.DB, story hn.Item) error {
	log.Println("Inserting story", story.ID)
	insertStorySQL := `INSERT INTO stories (id, by, title, url, timestamp) VALUES (?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`
	statement, err := db.Prepare(insertStorySQL) // Prepare statement.
	// This is good to avoid SQL injections
	if err != nil {
		return err
	}
	_, err = statement.Exec(story.ID, story.By, story.Title, story.URL, story.Timestamp)
	if err != nil {
		return err
	}
	return nil

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
		Timeout: time.Duration(60 * time.Second),
	})

	var ourMaxItem uint64 = 0

	row := db.QueryRow("select max(id) from stories")
	err = row.Scan(&ourMaxItem)

	fmt.Println("Got our max item", ourMaxItem)
	if ourMaxItem == 0 {
		panic("Failed to get ourMaxItem")
	}

	// Set up a ticker that periodically checks for the max
	// item ID and then downloads all items from the last one
	// we downloaded to that ID.
	ticker := time.NewTicker(5 * time.Second)
	quit := make(chan struct{})
	for {
		select {
		case <-ticker.C:

			maxItem, err := hn.Live.MaxItem()

			var theirMaxItem uint64 = uint64(maxItem)

			if err != nil {
				log.Fatal(err)
			}

			fmt.Println("Their max item", theirMaxItem)
			fmt.Println("We are", (theirMaxItem - ourMaxItem), "items behind")

			n := 100
			sem := make(chan struct{}, n)
			acquire := func() { sem <- struct{}{} }
			release := func() { <-sem }

			var wg sync.WaitGroup

			var nSuccess uint64 = 0

			for i := ourMaxItem + 1; i <= theirMaxItem; i++ {
				acquire()
				wg.Add(1)
				go func(id uint64) {
					defer release()
					defer wg.Done()
					item, err := hn.Item(int(id))
					if err != nil {
						log.Fatal(err)
					}
					// fmt.Println("Item type", id, item.Type)
					if item.Type == "story" {
						fmt.Println("Inserting story", item.ID)
						err := insertStory(db, *item)
						if err != nil {
							fmt.Println("failed to insert story", item.ID)
						} else {
							atomic.AddUint64(&nSuccess, 1)
							fmt.Println("Success", nSuccess)
						}
					} else {
						atomic.AddUint64(&nSuccess, 1)
					}
				}(i)
			}

			wg.Wait()

			// If we successfully inserted all items, update ourMaxItem so
			// next time we only start downloading items from tha tpoint. But
			// if there are any errors, start over.
			fmt.Println("Inserted nSuccess items", nSuccess, "out of ", (theirMaxItem - ourMaxItem))
			if nSuccess == (theirMaxItem - ourMaxItem) {
				ourMaxItem = theirMaxItem
			} else {
				fmt.Println("Didn't successfully insert all items. Will try again.")
				fmt.Println("ourMaxItem=", theirMaxItem)
			}

		case <-quit:
			ticker.Stop()
			return
		}
	}
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
