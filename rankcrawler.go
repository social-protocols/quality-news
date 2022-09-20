package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/johnwarden/hn"
)

type getStoriesFunc func() ([]int, error)

type ranksArray [5]int32

type dataPoint struct {
	id             int
	score          int
	descendants    int
	submissionTime int64
	sampleTime     int64
	ranks          ranksArray
}

func rankToNullableInt(rank int32) (result sql.NullInt32) {
	if rank == 0 {
		result = sql.NullInt32{}
	} else {
		result = sql.NullInt32{Int32: rank, Valid: true}

	}
	return
}

func rankCrawler(ndb newsDatabase, client *hn.Client) {
	ticker := time.NewTicker(60 * time.Second)
	quit := make(chan struct{})
	rankCrawlerStep(ndb, client)
	for {
		select {
		case <-ticker.C:
			rankCrawlerStep(ndb, client)

		case <-quit:
			ticker.Stop()
			return
		}
	}

}

func rankCrawlerStep(ndb newsDatabase, client *hn.Client) {

	sampleTime := time.Now().Unix()

	pageTypes := map[int]string{
		0: "top",
		1: "new",
		2: "best",
		3: "ask",
		4: "show",
	}

	ranksMap := map[int]ranksArray{}

	getKeys := func(m map[int]ranksArray) []int {
		keys := make([]int, len(m))
		i := 0
		for key := range m {
			keys[i] = key
			i++
		}
		return keys
	}

	// calculate ranks
	for pageType, pageTypeString := range pageTypes {
		ids, err := client.Stories(pageTypeString)
		if err != nil {
			log.Fatal(err)
		}

		for i, id := range ids {
			var ranks ranksArray
			var ok bool

			if ranks, ok = ranksMap[id]; !ok {
				ranks = ranksArray{}
			}

			ranks[pageType] = int32(i + 1)
			ranksMap[id] = ranks

			// only take the first 90 ranks
			if i+1 >= 90 {
				break
			}
		}
	}

	uniqueStoryIds := getKeys(ranksMap)
	const maxTries = 3
	const retryDelay = 10 * time.Second
	var tries int

TRIES:
	for tries < maxTries {
		// get story details
		fmt.Printf("Getting details for %d stories\n", len(uniqueStoryIds))

		items, err := client.GetItems(uniqueStoryIds)

		failedIDs := map[int]ranksArray{}
		if err != nil {
			fmt.Println("Failed to fetch some story IDs", err)

			for i, item := range items {
				// If item is empty
				if item.ID == 0 {
					failedIDs[uniqueStoryIds[i]] = ranksArray{}
				}
			}
		}

		log.Printf("Inserting rank data for %d items\n", len(items))
	ITEM:
		for _, item := range items {
			// Skip any items that were not fetched successfully.
			if item.ID == 0 {
				continue ITEM
			}
			storyID := item.ID
			ranks := ranksMap[storyID]

			datapoint := dataPoint{
				id:             storyID,
				score:          item.Score,
				descendants:    item.Descendants,
				submissionTime: int64(item.Time().Unix()),
				sampleTime:     sampleTime,
				ranks:          ranks,
			}
			err := ndb.insertDataPoint(datapoint)
			if err != nil {
				log.Fatal(err)
			}
			err = ndb.insertOrReplaceStory(item)
			if err != nil {
				log.Fatal(err)
			}
		}
		log.Printf("Successfully insertd rank data for %d items\n", len(items))

		if len(failedIDs) > 0 {
			uniqueStoryIds = getKeys(failedIDs)
			tries++
			fmt.Printf("Sleeping and then retrying (%d) %d stories\n", tries, len(uniqueStoryIds))
			time.Sleep(retryDelay)
			continue TRIES
		}

		// get details for every unique story
		break TRIES
	}
}
