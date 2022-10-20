package main

import (
	"database/sql"
	"time"

	"github.com/johnwarden/hn"

	"github.com/pkg/errors"
)

type getStoriesFunc func() ([]int, error)

type ranksArray [5]int

type dataPoint struct {
	id                        int
	score                     int
	descendants               int
	submissionTime            int64
	sampleTime                int64
	ranks                     ranksArray
	cumulativeExpectedUpvotes float64
	cumulativeUpvotes int
}

func rankToNullableInt(rank int) (result sql.NullInt32) {
	if rank == 0 {
		result = sql.NullInt32{}
	} else {
		result = sql.NullInt32{Int32: int32(rank), Valid: true}

	}
	return
}

func crawlHN(ndb newsDatabase, client *hn.Client, logger leveledLogger) error {

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
		var i int
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
			return errors.Wrap(err, "client.Stories")
		}

		for i, id := range ids {
			var ranks ranksArray
			var ok bool

			if ranks, ok = ranksMap[id]; !ok {
				ranks = ranksArray{}
			}

			ranks[pageType] = i + 1
			ranksMap[id] = ranks

			// only take the first 90 ranks
			if i+1 >= 90 {
				break
			}

		}
	}

	uniqueStoryIds := getKeys(ranksMap)

	// get story details
	logger.Info("Getting details for stories", "num_stories", len(uniqueStoryIds))

	items, err := client.GetItems(uniqueStoryIds)
	if err != nil {
		return (errors.Wrap(err, "client.GetItems"))
	}

	logger.Info("Inserting rank data", "nitems", len(items))
	// get details for every unique story

	var sitewideUpvotes int
	var deltaUpvotes = make([]int, len(items))
	var lastCumulativeExpectedUpvotes = make([]float64, len(items))
	var lastCumulativeUpvotes = make([]int, len(items))

ITEM:
	for i, item := range items {
		// Skip any items that were not fetched successfully.
		if item.ID == 0 {
			continue ITEM
		}
		storyID := item.ID

		{
			lastSeenScore, lastUpvotes, lastExpectedUpvotes, err := ndb.selectLastSeenScore(storyID)
			if err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					logger.Err(errors.Wrap(err, "selectLastSeenScore"))
				}
			} else {
				deltaUpvotes[i] = item.Score - lastSeenScore
				lastCumulativeExpectedUpvotes[i] = lastExpectedUpvotes
				lastCumulativeUpvotes[i] = lastUpvotes
			}
		}

		sitewideUpvotes += deltaUpvotes[i]

		if err := ndb.insertOrReplaceStory(item); err != nil {
			return errors.Wrap(err, "insertOrReplaceStory")
		}

	}

	logger.Debug("sitewideUpvotes", "value", sitewideUpvotes)

	var totalDeltaAttention float64
	var totalAttentionShare float64
	var deltaExpectedUpvotes float64
	for i, item := range items {

		storyID := item.ID
		ranks := ranksMap[storyID]
		deltaExpectedUpvotes = 0

	RANKS:
		for pageType, rank := range ranks {
			if rank == 0 {
				continue RANKS
			}
			d := accumulateAttention(ndb, logger, pageType, storyID, rank, sampleTime, deltaUpvotes[i], sitewideUpvotes)
			totalDeltaAttention += d[0]
			totalAttentionShare += d[1]
			deltaExpectedUpvotes += d[1]
		}

		submissionTime := int64(item.Time().Unix())
		datapoint := dataPoint{
			id:                        storyID,
			score:                     item.Score,
			descendants:               item.Descendants,
			submissionTime:            submissionTime,
			sampleTime:                sampleTime,
			ranks:                     ranks,
			cumulativeExpectedUpvotes: lastCumulativeExpectedUpvotes[i] + deltaExpectedUpvotes,
			cumulativeUpvotes: lastCumulativeUpvotes[i] + deltaUpvotes[i],
		}
		if err := ndb.insertDataPoint(datapoint); err != nil {
			return errors.Wrap(err, "insertDataPoint")
		}

	}

	logger.Debug("Totals",
		"deltaAttention", totalDeltaAttention,
		"sitewideUpvotes", sitewideUpvotes,
		"totalAttentionShare", totalAttentionShare,
		"dataPoints", len(items))

	logger.Info("Successfully inserted rank data", "nitems", len(items))

	return nil

}
