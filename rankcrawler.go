package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/johnwarden/hn"
)

type getStoriesFunc func() ([]int, error)

type StoryRanks [5]int32

type DataPoint struct {
	id             int
	score          int
	descendants    int
	submissionTime int64
	sampleTime     int64
	ranks          StoryRanks
}

func rankToNullableInt(rank int32) (result sql.NullInt32) {
	if rank == 0 {
		result = sql.NullInt32{0, false}
	} else {
		result = sql.NullInt32{rank, true}
	}
	return
}

func rankCrawler(db *sql.DB, client *hn.Client) {
	ticker := time.NewTicker(60 * time.Second)
	quit := make(chan struct{})
	rankCrawlerStep(db, client)
	for {
		select {
		case <-ticker.C:
			rankCrawlerStep(db, client)

		case <-quit:
			ticker.Stop()
			return
		}
	}
}

	func getKeys[T interface{}](m map[int]T) []int {
		keys := make([]int, len(m))
		i := 0
		for key, _ := range m {
			keys[i] = key
			i++
		}
		return keys
	}

func rankCrawlerStep(db *sql.DB, client *hn.Client) {

	sampleTime := time.Now().Unix()

	pageTypes := map[int]string{
		0: "top",
		1: "new",
		2: "best",
		3: "ask",
		4: "show",
	}

	storyRanksMap := map[int]StoryRanks{}

	// calculate ranks
	for pageType, pageTypeString := range pageTypes {
		ids, err := client.Stories(pageTypeString)
		if err != nil {
			log.Fatal(err)
		}

		for i, id := range ids {
			var storyRanks StoryRanks
			var ok bool

			if storyRanks, ok = storyRanksMap[id]; !ok {
				storyRanks = StoryRanks{}
			}

			storyRanks[pageType] = int32(i + 1)
			storyRanksMap[id] = storyRanks

			// only take the first 90 ranks
			if i+1 >= 90 {
				break
			}
		}
	}






	uniqueStoryIds := getKeys(storyRanksMap)
	const maxTries = 5
	const retryDelay = 60*time.Second
	var tries int

	TRIES: for tries < maxTries {
		// get story details
		fmt.Printf("Getting details for %d stories\n", len(uniqueStoryIds))

		items, err := client.GetItems(uniqueStoryIds)

		if err != nil {
			fmt.Println("Failed to fetch some story IDs",err)
			failedIDs := map[int]bool{}

			for i, item := range items {
				// If item is empty
				if item.ID == 0 {
					failedIDs[uniqueStoryIds[i]] = true
				}
			}

			uniqueStoryIds = getKeys(failedIDs)
			tries++;
			fmt.Printf("Sleeping and then retrying (%d) %d stories\n", tries, len(uniqueStoryIds))
			time.Sleep(time.Second*60)
			continue TRIES
		}

		log.Printf("Inserting rank data for %d items\n", len(storyRanksMap))
		for _, item := range items {
			storyID := item.ID
			ranks := storyRanksMap[storyID]

			datapoint := DataPoint{
				id:             storyID,
				score:          item.Score,
				descendants:    item.Descendants,
				submissionTime: int64(item.Time().Unix()),
				sampleTime:     sampleTime,
				ranks:          ranks,
			}
			err := insertDataPoint(db, datapoint)
			if err != nil {
				log.Fatal(err)
			}
		}
		// get details for every unique story
		break TRIES
	}
}

func insertDataPoint(db *sql.DB, d DataPoint) error {
	insertStorySQL := `INSERT INTO dataset (id, score, descendants, submissionTime, sampleTime, topRank, newRank, bestRank, askRank, showRank) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	statement, err := db.Prepare(insertStorySQL) // Prepare statement.

	if err != nil {
		return err
	}
	_, err = statement.Exec(d.id, d.score, d.descendants, d.submissionTime, d.sampleTime, rankToNullableInt(d.ranks[0]), rankToNullableInt(d.ranks[1]), rankToNullableInt(d.ranks[2]), rankToNullableInt(d.ranks[3]), rankToNullableInt(d.ranks[4]))
	if err != nil {
		return err
	}
	return nil
}
