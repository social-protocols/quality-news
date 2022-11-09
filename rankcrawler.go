package main

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
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
	// this function is called every minute. It crawls the hacker news
	// website, collects all stories which appear on a rank < 90 and stores
	// its ranks for all different pageTypes, and updates the story in the DB


	ndb := app.ndb
	logger := app.logger

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

	storyRanks := map[int]ranksArray{}
	stories := map[int]Story{}

	for pageType := 0; pageType < len(pageTypes); pageType++ {
		pageTypeName := pageTypes[pageType]

		nSuccess := 0

		resultCh := make(chan ScrapedStory)
		errCh := make(chan error)

		var wg sync.WaitGroup

		// scrape in a goroutine. the scraper will write results to the channel
		// we provide 
		wg.Add(1)
		go func() {
			defer wg.Done()
			app.scrapeHN(pageTypeName, resultCh, errCh)
		}()

		// read from the error channel in print errors in a separate goroutine.
		// The scraper will block writing to the error channel if nothing is reading
		// from it.
		wg.Add(1)
		go func() {
			defer wg.Done()
			for err := range errCh {
				app.logger.Err(errors.Wrap(err, "Error parsing story"))
				crawlErrorsTotal.Inc()
			}
		}()

		for result := range resultCh {
			oneBasedRank := result.Rank
			story := result.Story
			id := story.ID

			var ranks ranksArray
			var ok bool

			if ranks, ok = storyRanks[id]; !ok {
				// if story is not in storyRanks, initialize it with empty ranks
				ranks = ranksArray{}
			}

			ranks[pageType] = oneBasedRank
			storyRanks[id] = ranks
			stories[id] = story

			nSuccess += 1
		}

		logger.Debugf("Successfully crawled %d stories on %s page", nSuccess, pageTypeName)

		wg.Wait()

		// Sleep a bit to avoid rate limiting
		time.Sleep(time.Millisecond * 100)
	}


	uniqueStoryIds := getKeys(storyRanks)

	logger.Info("Inserting rank data", "nitems", len(uniqueStoryIds))

	// for every story, calculate metrices used for ranking
	var sitewideUpvotes int // total number of upvotes (since last sample point)
	// per story:
	deltaUpvotes := make([]int, len(uniqueStoryIds))          // number of upvotes (since last sample point)
	lastCumulativeUpvotes := make([]int, len(uniqueStoryIds)) // last number of upvotes tracked by our crawler
	lastCumulativeExpectedUpvotes := make([]float64, len(uniqueStoryIds))

STORY:
	for i, id := range uniqueStoryIds {
		story := stories[id]

		// Skip any stories that were not fetched successfully.
		if story.ID == 0 {
			logger.Error("Missing story id in story")
			crawlErrorsTotal.Inc()
			continue STORY
		}

		storyID := story.ID

		lastSeenScore, lastSeenUpvotes, lastSeenExpectedUpvotes, err := ndb.selectLastSeenData(tx, storyID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return errors.Wrap(err, "selectLastSeenScore")
			}
		} else {
			deltaUpvotes[i] = story.Upvotes + 1 - lastSeenScore
			lastCumulativeUpvotes[i] = lastSeenUpvotes
			lastCumulativeExpectedUpvotes[i] = lastSeenExpectedUpvotes
		}

		sitewideUpvotes += deltaUpvotes[i]

		// save story details in database
		if _, err := ndb.insertOrReplaceStory(tx, story); err != nil {
			return errors.Wrap(err, "insertOrReplaceStory")
		}
	}

	logger.Debug("sitewideUpvotes", "value", sitewideUpvotes)

	var totalDeltaExpectedUpvotes float64
	var totalExpectedUpvotesShare float64

	for i, id := range uniqueStoryIds {
		story := stories[id]

		ranks := storyRanks[id]
		expectedUpvotesAcrossPageTypes := 0.0

	RANKS:
		for pageType, rank := range ranks {
			if rank == 0 {
				continue RANKS
			}
			deltaExpectedUpvotes, expectedUpvotesShare := deltaExpectedUpvotes(ndb, logger, pageType, id, rank, sampleTime, deltaUpvotes[i], sitewideUpvotes)
			expectedUpvotesAcrossPageTypes += deltaExpectedUpvotes

			totalDeltaExpectedUpvotes += deltaExpectedUpvotes
			totalExpectedUpvotesShare += expectedUpvotesShare
		}

		submissionTime := int64(story.SubmissionTime)
		datapoint := dataPoint{
			id:                        id,
			score:                     story.Upvotes + 1,
			descendants:               story.Comments,
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
	logger.Info("Completed crawl", "nitems", len(stories), "elapsed", time.Since(t))

	upvotesTotal.Add(sitewideUpvotes)

	logger.Debug("Totals",
		"deltaExpectedUpvotes", totalDeltaExpectedUpvotes,
		"sitewideUpvotes", sitewideUpvotes,
		"totalExpectedUpvotesShare", totalExpectedUpvotesShare,
		"dataPoints", len(stories))


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
