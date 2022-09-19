package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	hn "github.com/peterhellberg/hn"
)

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

func storiesCrawler() {

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
	// Get the max item ID from the database. The crawler will pick
	// up from here.
	{
		row := db.QueryRow("select max(id) from stories")
		err = row.Scan(&ourMaxItem)

		fmt.Println("Got our max item", ourMaxItem)
		if ourMaxItem == 0 {
			panic("Failed to get ourMaxItem")
		}
	}

	// getLatestItems first queries the hacker news API for
	// theirMaxItem, the latest item ID. It then fetches all items between
	// ourLastItem and theirMaxItem. If successful, it updates ourMaxItem
	getLatestItems := func() {
		maxItem, err := hn.Live.MaxItem()

		if err != nil {
			log.Fatal(err)
		}

		var theirMaxItem uint64 = uint64(maxItem)

		fmt.Println("Their max item", theirMaxItem)
		fmt.Println("We are", (theirMaxItem - ourMaxItem), "items behind")

		// Use a channel as a rate limiter. If there are more than
		// nGoroutines running reading from the channel will block
		// until one goroutine releases.
		nGoroutines := 100
		sem := make(chan struct{}, nGoroutines)
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
					fmt.Println("failed to fetch story", item.ID, err)
					return
				}

				if item.Type == "story" {
					fmt.Println("Inserting story", item.ID)
					err := insertStory(db, *item)
					if err != nil {
						fmt.Println("failed to insert story", item.ID, err)
						return
					}
					atomic.AddUint64(&nSuccess, 1)
					fmt.Println("Success", nSuccess)
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

	}

	// Set up a ticker that periodically checks for the max
	// item ID and then downloads all items from the last one
	// we downloaded to that ID.
	ticker := time.NewTicker(5 * time.Second)
	quit := make(chan struct{})
	for {
		select {
		case <-ticker.C:

			getLatestItems()

		case <-quit:
			ticker.Stop()
			return
		}
	}
}
