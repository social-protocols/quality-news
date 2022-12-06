package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/exp/slog"
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

func (app app) crawlAndPostprocess(ctx context.Context) (err error) {
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

	var initialStoryCount, finalStoryCount, sitewideUpvotes int

	// Use the commit/rollback in a defer pattern described in:
	// https://stackoverflow.com/questions/16184238/database-sql-tx-detecting-commit-or-rollback
	defer func() {
		if err != nil {
			// https://go.dev/doc/database/execute-transactions
			// If the transaction succeeds, it will be committed before the function exits, making the deferred rollback call a no-op.
			logger.Debug("Rolling back transaction")
			e := tx.Rollback()
			crawlErrorsTotal.Inc()
			if e != nil {
				logger.Error("tx.Rollback", e)
			}
			return
		}
		logger.Debug("Commit transaction")
		err = tx.Commit() // here we are setting the return value err
		if err != nil {
			return
		}

		submissionsTotal.Add(finalStoryCount - initialStoryCount)
		upvotesTotal.Add(sitewideUpvotes)
	}()

	initialStoryCount, err = ndb.storyCount(tx)
	if err != nil {
		return errors.Wrap(err, "storyCount")
	}

	sitewideUpvotes, err = app.crawl(ctx, tx)
	if err != nil {
		return
	}

	finalStoryCount, err = ndb.storyCount(tx)
	if err != nil {
		return errors.Wrap(err, "storyCount")
	}

	err = app.crawlPostprocess(ctx, tx)

	return err
}

const maxGoroutines = 50

func (app app) crawl(ctx context.Context, tx *sql.Tx) (int, error) {
	ndb := app.ndb
	client := app.hnClient
	logger := app.logger

	t := time.Now()
	sampleTime := t.Unix()

	storyRanks, err := app.getRanksFromAPI(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "getRanksFromAPI")
	}

	// make sure we also get data for every story that was ranked on QN in the previous crawl
	idsFromPreviousCrawl, err := app.getQNTopFromPreviousCrawl(ctx, tx)
	if err != nil {
		return 0, errors.Wrap(err, "getIDSFromPreviousCrawl")
	}
	for _, id := range idsFromPreviousCrawl {
		if _, ok := storyRanks[id]; !ok {
			// create an empty ranks array for stories that were not ranked on
			// any of the HN pages but where ranked on QN in the last crawl
			storyRanks[id] = ranksArray{}
		}
	}

	stories, err := app.scrapeFrontPageStories(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "scrapeFrontPageStories")
	}

	uniqueStoryIds := getKeys(storyRanks)

	// Now use the API to get details for stories we did not find on the front page
	{
		missingStoryIDs := make([]int, 0, len(uniqueStoryIds))
		for _, id := range uniqueStoryIds {
			if _, ok := stories[id]; !ok {
				missingStoryIDs = append(missingStoryIDs, id)
			}
		}

		t := time.Now()

		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		// get story details
		logger.Info("Getting story details from API for stories that were not on the front page", "num_stories", len(uniqueStoryIds), "missing_stories", len(missingStoryIDs))
		missingStories, err := client.GetItems(ctx, missingStoryIDs, maxGoroutines)
		if err != nil {
			return 0, errors.Wrap(err, "client.GetItems")
		}

		if len(missingStoryIDs) != len(missingStories) {
			panic(fmt.Sprintf("Story counts don't add up after downloading missing stories: %d, %d", len(missingStoryIDs), len(missingStories)))
		}

		for _, s := range missingStories {
			stories[s.ID] = ScrapedStory{
				Story: Story{
					ID:                     s.ID,
					By:                     s.By,
					Title:                  s.Title,
					URL:                    s.URL,
					SubmissionTime:         int64(s.Timestamp),
					OriginalSubmissionTime: int64(s.Timestamp),
					AgeApprox:              sampleTime - int64(s.Timestamp),
					Score:                  s.Score,
					Comments:               s.Descendants,
				},
				Source: "api",
			}
		}

		// Output some errors if there are inconsistencies between the set of story IDs the API tells us are on top page
		// and the set of stories we were able to fetch details for from the API or the scraper
		if len(uniqueStoryIds) != len(getKeys(stories)) {
			for _, id := range uniqueStoryIds {
				if _, ok := stories[id]; !ok {
					logger.Warn("failed to get story details for story", "story_id", id)
				}
			}
			for id := range stories {
				if _, ok := storyRanks[id]; !ok {
					logger.Warn("found story on top page from scraper but not on top page from API", "story_id", id)
				}
			}
		}

		logger.Info("Got story details from API", "nitems", len(missingStoryIDs), slog.Duration("elapsed", time.Since(t)))
	}

	// for every story, calculate metrics used for ranking per story:
	var sitewideUpvotes int
	deltaUpvotes := make([]int, len(uniqueStoryIds))          // number of upvotes (since last sample point)
	lastCumulativeUpvotes := make([]int, len(uniqueStoryIds)) // last number of upvotes tracked by our crawler
	lastCumulativeExpectedUpvotes := make([]float64, len(uniqueStoryIds))
	lastSeenTimes := make([]int, len(uniqueStoryIds))

	logger.Info("Inserting stories into DB", "nitems", len(uniqueStoryIds))

	// insert stories into DB and update aggregate metrics
STORY:
	for i, id := range uniqueStoryIds {
		story := stories[id]

		// Skip any stories that were not fetched successfully.
		if story.ID == 0 {
			logger.Error("Missing story id in story", nil)
			crawlErrorsTotal.Inc()
			continue STORY
		}

		storyID := story.ID

		lastSeenScore, lastSeenUpvotes, lastSeenExpectedUpvotes, lastSeenTime, err := ndb.selectLastSeenData(tx, storyID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return 0, errors.Wrap(err, "selectLastSeenScore")
			}
		} else {
			// if no more than 5 minutes have passed.
			lastCumulativeUpvotes[i] = lastSeenUpvotes
			lastCumulativeExpectedUpvotes[i] = lastSeenExpectedUpvotes
			lastSeenTimes[i] = lastSeenTime

			deltaUpvotes[i] = story.Score - lastSeenScore
			sitewideUpvotes += deltaUpvotes[i]

		}

		// save story details in database
		if _, err := ndb.insertOrReplaceStory(tx, story.Story); err != nil {
			return 0, errors.Wrap(err, "insertOrReplaceStory")
		}
	}

	logger.Info("Inserting rank data into DB", "nitems", len(uniqueStoryIds))

	var sitewideDeltaExpectedUpvotes float64
	var sitewideExpectedUpvotesShare float64

	for i, id := range uniqueStoryIds {
		story := stories[id]

		ranks := storyRanks[id]
		cumulativeUpvotes := lastCumulativeUpvotes[i]
		cumulativeExpectedUpvotes := lastCumulativeExpectedUpvotes[i]

		if sampleTime-int64(lastSeenTimes[i]) < 300 {
			// only accumulate upvotes if we haven't gone more than a few
			// minutes since last crawl. Otherwise our assumption that the
			// story has been at this rank since the last crawl starts to
			// become less and less reasonable

			cumulativeUpvotes += deltaUpvotes[i]

		RANKS:
			for pageType, rank := range ranks {
				if rank == 0 {
					continue RANKS
				}

				expectedUpvotesShare := expectedUpvoteShare(pageType, rank)
				deltaExpectedUpvotes := expectedUpvotesShare * float64(sitewideUpvotes)

				cumulativeExpectedUpvotes += deltaExpectedUpvotes
				sitewideDeltaExpectedUpvotes += deltaExpectedUpvotes
				sitewideExpectedUpvotesShare += expectedUpvotesShare
			}
		}

		datapoint := dataPoint{
			id:                        id,
			score:                     story.Score,
			descendants:               story.Comments,
			sampleTime:                sampleTime,
			submissionTime:            story.SubmissionTime,
			ranks:                     ranks,
			cumulativeExpectedUpvotes: cumulativeExpectedUpvotes,
			cumulativeUpvotes:         cumulativeUpvotes,
			ageApprox:                 story.AgeApprox,
			flagged:                   story.Flagged,
			job:                       story.Job,
		}

		if err := ndb.insertDataPoint(tx, datapoint); err != nil {
			return sitewideUpvotes, errors.Wrap(err, "insertDataPoint")
		}
	}

	crawlDuration.UpdateDuration(t)
	logger.Info("Finished crawl",
		"nitems", len(stories), slog.Duration("elapsed", time.Since(t)),
		"deltaExpectedUpvotes", sitewideDeltaExpectedUpvotes,
		"sitewideUpvotes", sitewideUpvotes,
		"sitewideExpectedUpvotesShare", sitewideExpectedUpvotesShare,
		"dataPoints", len(stories))

	return sitewideUpvotes, nil
}

// getRanksFromAPI gets all ranks for all page types from the API and puts them into
// a map[int]ranksArray
func (app app) getRanksFromAPI(ctx context.Context) (map[int]ranksArray, error) {
	app.logger.Info("Getting ranks from API")

	t := time.Now()

	storyRanks := map[int]ranksArray{}

	client := app.hnClient

	for pageType := 0; pageType < len(pageTypes); pageType++ {
		pageTypeName := pageTypes[pageType]

		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		storyIDs, err := client.Stories(ctx, pageTypeName)
		if err != nil {
			return storyRanks, errors.Wrap(err, "client.Stories")
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

	app.logger.Info("Got ranks from api", slog.Duration("elapsed", time.Since(t)))

	return storyRanks, nil
}

func (app app) getQNTopFromPreviousCrawl(ctx context.Context, tx *sql.Tx) ([]int, error) {
	result := make([]int, 0, 90)

	s, err := tx.Prepare("select id from dataset where qnRank <= 90 and sampleTime = (select max(sampleTime) from dataset where sampleTime != (select max(sampleTime) from dataset))")
	if err != nil {
		return nil, errors.Wrap(err, "preparing getQNTopFromPreviousCrawl sql")
	}

	rows, err := s.QueryContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "executing getQNTopFromPreviousCrawl sql")
	}
	defer rows.Close()

	for rows.Next() {

		var id sql.NullInt32

		err := rows.Scan(&id)
		if err != nil {
			return nil, errors.Wrap(err, "rows.Scan")
		}
		result = append(result, int(id.Int32))
	}

	return result, nil
}

func (app app) crawlPostprocess(ctx context.Context, tx *sql.Tx) error {
	t := time.Now()

	err := app.updateResubmissions(ctx, tx)
	if err != nil {
		return errors.Wrap(err, "updateResubmissions")
	}

	err = app.updatePenalties(ctx, tx)
	if err != nil {
		return errors.Wrap(err, "estimatePenalties")
	}

	err = app.updateQNRanks(ctx, tx)

	crawlPostprocessingDuration.UpdateDuration(t)

	app.logger.Info("Finished crawl postprocessing", slog.Duration("elapsed", time.Since(t)))

	return errors.Wrap(err, "update QN Ranks")
}

const qnRankFormulaSQL = "pow((cumulativeUpvotes + overallPriorWeight)/(cumulativeExpectedUpvotes + overallPriorWeight) * ageHours, 0.8) / pow(ageHours + 2, gravity) desc"

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

	app.logger.Debug("Finished executing updateQNRanks", slog.Duration("elapsed", time.Since(t)))

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

	app.logger.Debug("Finished executing resubmissions", slog.Duration("elapsed", time.Since(t)))

	return errors.Wrap(err, "executing resubmissions SQL")
}

var penaltiesSQL = readSQLSource("penalties.sql")

func (app app) updatePenalties(ctx context.Context, tx *sql.Tx) error {
	t := time.Now()

	stmt, err := tx.Prepare(penaltiesSQL)
	if err != nil {
		return errors.Wrap(err, "preparing penaltiesSQL")
	}

	_, err = stmt.ExecContext(ctx)

	app.logger.Debug("Finished executing penpenaltiesSQLalties", slog.Duration("elapsed", time.Since(t)))

	return errors.Wrap(err, "executing penaltiesSQL")
}
