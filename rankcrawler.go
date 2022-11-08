package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pkg/errors"
)

const maxGoroutines = 20

var pageTypes = map[int]string{
	0: "top",
	1: "new",
	2: "best",
	3: "ask",
	4: "show",
}

type ranksArray [5]int // the ranks of a story for different pageTypes

type dataPoint struct {
	// One datapoint represents the state of a single story at a specific point in time.
	// It is one row of the `dataset` table.
	id                        int
	score                     int
	descendants               int
	submissionTime            int64
	sampleTime                int64
	ranks                     ranksArray
	cumulativeExpectedUpvotes float64
	cumulativeUpvotes         int
}

func (app app) crawlHN(ctx context.Context) (err error) {
	// this function is called every minute.
	// It queries all storyIDs from all pageTypes from the Hacker News API.
	// Collect all stories which appear on a rank < 90 and
	// store its ranks for all different pageTypes.
	// For every resulting story request all details from the Hacker News API.
	// With this data, we can compute the expected upvotes per story.

	ndb := app.ndb
	logger := app.logger
	client := app.hnClient

	tx, e := ndb.db.BeginTx(ctx, nil)
	if e != nil {
		err = errors.Wrap(e, "BeginTX")
		crawlErrorsTotal.Inc()
		return
	}

	// Use the commit/rollback in a defer pattern described in:
	// https://stackoverflow.com/questions/16184238/database-sql-tx-detecting-commit-or-rollback
	defer func() {
		if err != nil {
			// https://go.dev/doc/database/execute-transactions
			// If the transaction succeeds, it will be committed before the function exits, making the deferred rollback call a no-op.
			tx.Rollback()
			return
		}
		err = tx.Commit()
	}()

	t := time.Now()
	sampleTime := t.Unix()

	// calculate ranks for every story
	storyRanks := map[int]ranksArray{}
	for pageType, pageTypeName := range pageTypes {
		storyIDs, err := client.Stories(ctx, pageTypeName)
		if err != nil {
			return errors.Wrap(err, "client.Stories")
		}

		for zeroBasedRank, ID := range storyIDs {
			var ranks ranksArray
			var ok bool

			if ranks, ok = storyRanks[ID]; !ok {
				// if story is not in storyRanks, initialize it with empty ranks
				ranks = ranksArray{}
			}

			ranks[pageType] = zeroBasedRank + 1
			storyRanks[ID] = ranks

			// only take stories which appear on the first 90 ranks
			if zeroBasedRank+1 >= 90 {
				break
			}

		}
	}

	uniqueStoryIds := getKeys(storyRanks)

	// get story details
	logger.Info("Getting details for stories", "num_stories", len(uniqueStoryIds))
	stories, err := client.GetItems(ctx, uniqueStoryIds, maxGoroutines)
	if err != nil {
		return errors.Wrap(err, "client.GetItems")
	}

	logger.Info("Inserting rank data", "nitems", len(stories))
	// get details for every unique story

	// for every story, calculate metrices used for ranking
	var sitewideUpvotes int // total number of upvotes (since last sample point)
	// per story:
	deltaUpvotes := make([]int, len(stories))          // number of upvotes (since last sample point)
	lastCumulativeUpvotes := make([]int, len(stories)) // last number of upvotes tracked by our crawler
	lastCumulativeExpectedUpvotes := make([]float64, len(stories))

STORY:
	for i, item := range stories {
		// Skip any stories that were not fetched successfully.
		if item.ID == 0 {
			continue STORY
		}

		storyID := item.ID

		lastSeenScore, lastSeenUpvotes, lastSeenExpectedUpvotes, err := ndb.selectLastSeenData(tx, storyID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return errors.Wrap(err, "selectLastSeenScore")
			}
		} else {
			deltaUpvotes[i] = item.Score - lastSeenScore
			lastCumulativeUpvotes[i] = lastSeenUpvotes
			lastCumulativeExpectedUpvotes[i] = lastSeenExpectedUpvotes
		}

		sitewideUpvotes += deltaUpvotes[i]

		// save story details in database
		if _, err := ndb.insertOrReplaceStory(tx, item); err != nil {
			return errors.Wrap(err, "insertOrReplaceStory")
		}
	}

	logger.Debug("sitewideUpvotes", "value", sitewideUpvotes)

	var totalDeltaExpectedUpvotes float64
	var totalExpectedUpvotesShare float64

	for i, item := range stories {
		storyID := item.ID
		ranks := storyRanks[storyID]
		expectedUpvotesAcrossPageTypes := 0.0

	RANKS:
		for pageType, rank := range ranks {
			if rank == 0 {
				continue RANKS
			}
			deltaExpectedUpvotes, expectedUpvotesShare := deltaExpectedUpvotes(ndb, logger, pageType, storyID, rank, sampleTime, deltaUpvotes[i], sitewideUpvotes)
			expectedUpvotesAcrossPageTypes += deltaExpectedUpvotes

			totalDeltaExpectedUpvotes += deltaExpectedUpvotes
			totalExpectedUpvotesShare += expectedUpvotesShare
		}

		submissionTime := int64(item.Time().Unix())
		datapoint := dataPoint{
			id:                        storyID,
			score:                     item.Score,
			descendants:               item.Descendants,
			submissionTime:            submissionTime,
			sampleTime:                sampleTime,
			ranks:                     ranks,
			cumulativeExpectedUpvotes: lastCumulativeExpectedUpvotes[i] + expectedUpvotesAcrossPageTypes,
			cumulativeUpvotes:         lastCumulativeUpvotes[i] + deltaUpvotes[i],
		}
		if err := ndb.insertDataPoint(tx, datapoint); err != nil {
			return errors.Wrap(err, "insertDataPoint")
		}
	}

	crawlDuration.UpdateDuration(t)
	upvotesTotal.Add(sitewideUpvotes)

	logger.Debug("Totals",
		"deltaExpectedUpvotes", totalDeltaExpectedUpvotes,
		"sitewideUpvotes", sitewideUpvotes,
		"totalExpectedUpvotesShare", totalExpectedUpvotesShare,
		"dataPoints", len(stories))

	logger.Info("Successfully inserted rank data", "nitems", len(stories))

	err = app.updateQNRanks(ctx, tx)
	return errors.Wrap(err, "update QN Ranks")
}

const updateQNRanksSQL = `
	with parameters as (select %f as priorWeight, %f as overallPriorWeight, %f as gravity)
	, latestData as (
		select	
			id
			, score
			, sampleTime
			, cast(sampleTime-submissionTime as real)/3600 as ageHours
			, cumulativeUpvotes
			, cumulativeExpectedUpvotes
		from dataset 
		where sampleTime = (select max(sampleTime) from dataset)
	),
	qnRanks as (
		select 
		id
			, dense_rank() over(order by %s) as rank
			, sampleTime
		from latestData join parameters
	)
	update dataset as d set qnRank = qnRanks.rank
	from qnRanks
	where d.id = qnRanks.id and d.sampleTime = qnRanks.sampleTime;
`

func (app app) updateQNRanks(ctx context.Context, tx *sql.Tx) error {
	gravity := defaultFrontPageParams.Gravity
	overallPriorWeight := defaultFrontPageParams.OverallPriorWeight
	priorWeight := defaultFrontPageParams.PriorWeight

	sql := fmt.Sprintf(updateQNRanksSQL, priorWeight, overallPriorWeight, gravity, qnRankFormulaSQL)
	stmt, err := tx.Prepare(sql)
	if err != nil {
		return errors.Wrap(err, "preparing updateQNRanksSQL")
	}

	_, err = stmt.ExecContext(ctx)

	return errors.Wrap(err, "executing updateQNRanksSQL")
}
