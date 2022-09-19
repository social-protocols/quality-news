package main

import (
	"database/sql"
	"log"
	"time"

	"github.com/peterhellberg/hn"
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

func rankCrawler(db *sql.DB, hn *hn.Client) {

	sampleTime := time.Now().Unix()

	pageTypes := map[int]string{
		0: "top",
		1: "new",
		2: "best",
		3: "ask",
		4: "show",
	}

	storyRanksMap := map[int]StoryRanks{}

	for pageType, pageTypeString := range pageTypes {
		ids := hn.Stories(pageTypeString)

		for i, id := range ids {
			var storyRanks StoryRanks
			var ok bool

			if storyRanks, ok = storyRanksMap[id]; !ok {
				storyRanks = StoryRanks{}
			}

			storyRanks[pageType] = i + 1

			storyRanksMap[id] = storyRanks
		}
	}

	for storyId, ranks := range storyRanksMap {
		item, err := hn.Item(storyId)
		if err != nil {
			log.Fatal(err)
		}

		datapoint := DataPoint{
			id:             storyId,
			score:          item.Score,
			descendants:    item.Descendants,
			submissionTime: int64(item.Time().Second()),
			sampleTime:     sampleTime,
			ranks:          ranks,
		}
		insertDataPoint(db, datapoint)
	}
	// get details for every unique story
}

func insertDataPoint(db *sql.DB, d DataPoint) error {
	log.Println("Inserting rank data point", d.id)
	insertStorySQL := `INSERT INTO dataset (id, score, descendants, submissionTime, sampleTime, topRank, newRank, bestRank, askRank, showRank) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	statement, err := db.Prepare(insertStorySQL) // Prepare statement.

	if err != nil {
		return err
	}
	_, err = statement.Exec(d.id, d.score, d.descendants, d.submissionTime, d.sampleTime, d.ranks[0], d.ranks[1], d.ranks[2], d.ranks[3], d.ranks[4])
	if err != nil {
		return err
	}
	return nil
}
