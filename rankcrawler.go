package main

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/exp/slog"
)

type pageTypeInt int

var (
	top  pageTypeInt = 0
	new  pageTypeInt = 1
	best pageTypeInt = 2
	ask  pageTypeInt = 3
	show pageTypeInt = 4
)

var pageTypes = map[pageTypeInt]string{
	top:  "top",
	new:  "new",
	best: "best",
	ask:  "ask",
	show: "show",
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
	dupe                      bool
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
	defer crawlDuration.UpdateDuration(t)
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

		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
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

	newRankChanges := make([]int, 0, 10)

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

			if story.SubmissionTime == 0 {
				panic(story)
			}

			if storyRanks[story.ID][1] != 0 {
				newRankChanges = append(newRankChanges, int(sampleTime)-int(story.SubmissionTime))
				lastSeenTimes[i] = int(story.SubmissionTime)
			}

		} else {
			lastCumulativeUpvotes[i] = lastSeenUpvotes
			lastCumulativeExpectedUpvotes[i] = lastSeenExpectedUpvotes
			lastSeenTimes[i] = lastSeenTime

			deltaUpvotes[i] = story.Score - lastSeenScore
			sitewideUpvotes += deltaUpvotes[i]

		}

		// save story details in database
		_, err = ndb.insertOrReplaceStory(tx, story.Story)
		if err != nil {
			return 0, errors.Wrap(err, "insertOrReplaceStory")
		}
	}

	logger.Info("Inserting rank data into DB", "nitems", len(uniqueStoryIds))

	var sitewideDeltaExpectedUpvotes float64
	var sitewideExpectedUpvotesShare float64

	if len(newRankChanges) > 0 {
		// If there have been N new submissions, each story above rank N has occupied N+1 ranks
		// So add a rank change time corresponding to the beginning of the crawl period.

		sort.Ints(newRankChanges)
	}

	for i, id := range uniqueStoryIds {
		story := stories[id]

		ranks := storyRanks[id]
		cumulativeUpvotes := lastCumulativeUpvotes[i]
		cumulativeExpectedUpvotes := lastCumulativeExpectedUpvotes[i]

		elapsedTime := int(sampleTime) - lastSeenTimes[i]

		if elapsedTime < 120 {
			// only accumulate upvotes if we haven't gone more than 2
			// minutes since last crawl. Otherwise our assumption that the
			// story has been at this rank since the last crawl starts to
			// become less and less reasonable

			cumulativeUpvotes += deltaUpvotes[i]

		RANKS:
			for pt, rank := range ranks {
				pageType := pageTypeInt(pt)
				if rank == 0 {
					continue RANKS
				}

				exUpvoteShare := 0.0

				if pageType == new && len(newRankChanges) > 0 {
					exUpvoteShare = expectedUpvoteShareNewPage(rank, elapsedTime, newRankChanges)
				} else {
					exUpvoteShare = expectedUpvoteShare(pageType, rank)
				}

				deltaExpectedUpvotes := exUpvoteShare * float64(sitewideUpvotes)

				cumulativeExpectedUpvotes += deltaExpectedUpvotes
				sitewideDeltaExpectedUpvotes += deltaExpectedUpvotes
				sitewideExpectedUpvotesShare += exUpvoteShare
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
			dupe:                      story.Dupe,
		}

		if err := ndb.insertDataPoint(tx, datapoint); err != nil {
			return sitewideUpvotes, errors.Wrap(err, "insertDataPoint")
		}
	}

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

	for pageType := top; pageType <= show; pageType++ {
		pageTypeName := pageTypes[pageType]

		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
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
