package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	hn "github.com/johnwarden/hn"
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

func makeRange(min, max int) []int {
	a := make([]int, max-min+1)
	for i := range a {
		a[i] = min + i
	}
	return a
}

func storiesCrawler(db *sql.DB, hnclient *hn.Client) {

	sqliteDataDir := os.Getenv("SQLITE_DATA_DIR")
	if sqliteDataDir == "" {
		panic("SQLITE_DATA_DIR not set")
	}

	frontpageDatabaseFilename := fmt.Sprintf("%s/frontpage.sqlite", sqliteDataDir)
	fmt.Println("Database file", frontpageDatabaseFilename)

	var ourMaxItem int
	// Get the max item ID from the database. The crawler will pick
	// up from here.
	{
		row := db.QueryRow("select max(id) from stories")
		err := row.Scan(&ourMaxItem)
		if err != nil {
			panic("Failed to get ourMaxItem")
		}

		// TODO:
		fmt.Println("Got our max item", ourMaxItem)
	}

	// getLatestItems first queries the hacker news API for
	// theirMaxItem, the latest item ID. It then fetches all items between
	// ourLastItem and theirMaxItem. If successful, it updates ourMaxItem
	getLatestItems := func() {
		fmt.Println("Getting the max itemID from the API")
		theirMaxItem, err := hnclient.Live.MaxItem()
		if err != nil {
			fmt.Println("Failed to get MaxItem", err)
			return
		}

		if ourMaxItem == 0 {
			fmt.Println("No max item in our database. Starting with ", theirMaxItem)
			ourMaxItem = theirMaxItem
		}

		fmt.Println("Their max item", theirMaxItem)
		fmt.Println("We are", (theirMaxItem - ourMaxItem), "items behind")

		items, err := hnclient.GetItems(makeRange(ourMaxItem+1, theirMaxItem))
		if err != nil {
			fmt.Println("GetItems failed", err)
			return
		}

		// No insert all the stories

		for _, item := range items {
			if item.Type == "story" {
				fmt.Println("Inserting story", item.ID)
				err := insertStory(db, item)
				if err != nil {
					fmt.Println("failed to insert story", item.ID, err)
					return
				}
			}
		}

		// If we successfully inserted all items, update ourMaxItem so
		// next time we only start downloading items from tha tpoint. But
		// if there are any errors, start over.

		ourMaxItem = theirMaxItem
		fmt.Println("ourMaxItem=", theirMaxItem)
		return
	}

	getLatestItems()

	ticker := time.NewTicker(5 * time.Second)
	// quit := make(chan struct{})
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
