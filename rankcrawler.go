package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/pkg/errors"
)

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
	sampleTime                int64
	submissionTime            int64
	ageApprox                 int64
	ranks                     ranksArray
	cumulativeExpectedUpvotes float64
	cumulativeUpvotes         int
	flagged                   bool
	job                       bool
}

func (app app) crawlHN(ctx context.Context) (count int, err error) {
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

	initialStoryCount, err := ndb.storyCount(tx)
	if err != nil {
		return 0, errors.Wrap(err, "storyCount")
	}

	// Use the commit/rollback in a defer pattern described in:
	// https://stackoverflow.com/questions/16184238/database-sql-tx-detecting-commit-or-rollback
	defer func() {
		if err != nil {
			// https://go.dev/doc/database/execute-transactions
			// If the transaction succeeds, it will be committed before the function exits, making the deferred rollback call a no-op.
			logger.Debug("Rolling back transaction")
			e := tx.Rollback()
			if e != nil {
				logger.Err(errors.Wrap(e, "tx.Rollback"))
			}
			return
		} else {
			logger.Debug("Commit transaction")
			err = tx.Commit()
		}
	}()

	t := time.Now()
	sampleTime := t.Unix()

	storyRanks := map[int]ranksArray{}
	stories := map[int]ScrapedStory{}

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

		for story := range resultCh {
			oneBasedRank := story.Rank
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

		if nSuccess == 0 {
			return 0, fmt.Errorf("Didn't successfully parse any stories from %s page", pageTypeName)
		}
		logger.Debugf("Crawled %d stories on %s page", nSuccess, pageTypeName)

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
				return 0, errors.Wrap(err, "selectLastSeenScore")
			}
		} else {
			deltaUpvotes[i] = story.Score - lastSeenScore
			lastCumulativeUpvotes[i] = lastSeenUpvotes
			lastCumulativeExpectedUpvotes[i] = lastSeenExpectedUpvotes
		}

		sitewideUpvotes += deltaUpvotes[i]

		// save story details in database
		if _, err := ndb.insertOrReplaceStory(tx, story.Story); err != nil {
			return 0, errors.Wrap(err, "insertOrReplaceStory")
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

		datapoint := dataPoint{
			id:                        id,
			score:                     story.Score,
			descendants:               story.Comments,
			sampleTime:                sampleTime,
			submissionTime:            story.SubmissionTime,
			ranks:                     ranks,
			cumulativeExpectedUpvotes: lastCumulativeExpectedUpvotes[i] + expectedUpvotesAcrossPageTypes,
			cumulativeUpvotes:         lastCumulativeUpvotes[i] + deltaUpvotes[i],
			ageApprox:                 story.AgeApprox,
			flagged:                   story.Flagged,
			job:                       story.Job,
		}

		if err := ndb.insertDataPoint(tx, datapoint); err != nil {
			return 0, errors.Wrap(err, "insertDataPoint")
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

	finalStoryCount, err := ndb.storyCount(tx)
	if err != nil {
		return 0, errors.Wrap(err, "storyCount")
	}

	err = app.updateResubmissions(ctx, tx)
	if err != nil {
		return 0, errors.Wrap(err, "updateResubmissions")
	}

	err = app.updatePenalties(ctx, tx)
	if err != nil {
		return 0, errors.Wrap(err, "estimatePenalties")
	}

	err = app.updateQNRanks(ctx, tx)
	return finalStoryCount - initialStoryCount, errors.Wrap(err, "update QN Ranks")
}

const qnRankFormulaSQL = "pow((cumulativeUpvotes + overallPriorWeight)/(cumulativeExpectedUpvotes + overallPriorWeight) * ageHours, 0.8) / pow(ageHours + 2, gravity) * (1 - penalty*penaltyWeight) desc"

func readSQLSource(filename string) string {
	f, err := resources.Open("sql/" + filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	buf := bytes.NewBuffer(nil)
	_, err = io.Copy(buf, f)
	if err != nil {
		panic(err)
	}

	return buf.String()
}

var qnRanksSQL = readSQLSource("qnranks.sql")

func (app app) updateQNRanks(ctx context.Context, tx *sql.Tx) error {

	t := time.Now()

	d := defaultFrontPageParams
	sql := fmt.Sprintf(qnRanksSQL, d.PriorWeight, d.OverallPriorWeight, d.Gravity, d.PenaltyWeight, qnRankFormulaSQL)

	stmt, err := tx.Prepare(sql)
	if err != nil {
		return errors.Wrap(err, "preparing updateQNRanksSQL")
	}

	_, err = stmt.ExecContext(ctx)

	app.logger.Info("Finished executing updateQNRanks", "elapsed ", time.Since(t))

	return errors.Wrap(err, "executing updateQNRanksSQL")
}

var resubmissionsSQL = readSQLSource("resubmissions.sql")

func (app app) updateResubmissions(ctx context.Context, tx *sql.Tx) error {

	t := time.Now()

	stmt, err := tx.Prepare(resubmissionsSQL)
	if err != nil {
		return errors.Wrap(err, "preparing resubmissions SQL")
	}

	_, err = stmt.ExecContext(ctx)

	app.logger.Info("Finished executing resubmissions", "elapsed ", time.Since(t))

	return errors.Wrap(err, "executing resubmissions SQL")
}

var penaltiesSQL = readSQLSource("penalties.sql")

func (app app) updatePenalties(ctx context.Context, tx *sql.Tx) error {

	t := time.Now()

	stmt, err := tx.Prepare(penaltiesSQL)
	if err != nil {
		return errors.Wrap(err, "preparing penalties SQL")
	}

	_, err = stmt.ExecContext(ctx)

	app.logger.Info("Finished executing penalties", "elapsed ", time.Since(t))

	return errors.Wrap(err, "executing penalties SQL")
}
