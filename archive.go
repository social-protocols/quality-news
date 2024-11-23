package main

import (
	"context"
	"encoding/json"
	"fmt"
	pond "github.com/alitto/pond/v2"
	"github.com/pkg/errors"
	"net/http"
)

type StatsData struct {
	RanksPlotData   [][]any `json:"RanksPlotData"`
	UpvotesPlotData [][]any `json:"UpvotesPlotData"`
	PenaltyPlotData [][]any `json:"PenaltyPlotData"`
	MaxSampleTime   int     `json:"MaxSampleTime"`
	SubmissionTime  int64   `json:"SubmissionTime"`
}

type responseBuffer struct {
	header http.Header
	body   []byte
	status int
}

func newResponseBuffer() *responseBuffer {
	return &responseBuffer{
		header: make(http.Header),
		status: http.StatusOK,
	}
}

func (r *responseBuffer) Header() http.Header {
	return r.header
}

func (r *responseBuffer) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return len(b), nil
}

func (r *responseBuffer) WriteHeader(statusCode int) {
	r.status = statusCode
}

func (app app) generateStatsDataJSON(ctx context.Context, storyID int) ([]byte, error) {
	ndb := app.ndb
	modelParams := OptionalModelParams{}.WithDefaults()

	// Fetch MaxSampleTime
	maxSampleTime, err := maxSampleTime(ndb, storyID)
	if err != nil {
		return nil, errors.Wrap(err, "maxSampleTime")
	}

	// Fetch RanksPlotData
	ranksPlotData, err := rankDatapoints(ndb, storyID)
	if err != nil {
		return nil, errors.Wrap(err, "rankDatapoints")
	}

	// Fetch UpvotesPlotData
	upvotesPlotData, err := upvotesDatapoints(ndb, storyID, modelParams)
	if err != nil {
		return nil, errors.Wrap(err, "upvotesDatapoints")
	}

	// Fetch PenaltyPlotData if needed
	penaltyPlotData := [][]any{} // Replace with actual data fetching if necessary

	// Fetch Story details
	s, err := ndb.selectStoryDetails(storyID)
	if err != nil {
		return nil, errors.Wrap(err, "selectStoryDetails")
	}

	// Create StatsData struct with story details
	statsData := StatsData{
		RanksPlotData:   ranksPlotData,
		UpvotesPlotData: upvotesPlotData,
		PenaltyPlotData: penaltyPlotData,
		MaxSampleTime:   maxSampleTime,
		SubmissionTime:  s.SubmissionTime,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(statsData)
	if err != nil {
		return nil, errors.Wrap(err, "json.Marshal statsData")
	}

	return jsonData, nil
}

func (app app) archiveOldStatsData(ctx context.Context) error {
	app.logger.Info("Looking for old stories to archive")
	app.logger.Debug("selectStoriesToArchive")
	storyIDs, err := app.ndb.selectStoriesToArchive(ctx)
	if err != nil {
		return errors.Wrap(err, "selectStoriesToArchive")
	}
	app.logger.Debug("Finished selectStoriesToArchive")

	if len(storyIDs) == 0 {
		return nil // Nothing to archive
	}

	app.logger.Info("Found old stories to archive", "nStories", len(storyIDs))

	sc, err := NewStorageClient()
	if err != nil {
		return errors.Wrap(err, "create storage client")
	}

	// Create a pool of workers to upload stories to archive in parallel
	pool := pond.NewPool(10, pond.WithContext(ctx))

	app.logger.Debug("Uploading archive JSON files")
	for _, storyID := range storyIDs {
		pool.Submit(func() {
			app.logger.Info("Archiving stats data for story", "storyID", storyID)
			err := app.archiveStory(ctx, sc, storyID)
			if err != nil {
				app.logger.Error("archiveStory", err)
			}
		})
	}

	// Wait for all tasks in the group to complete or the timeout to occur, whichever comes first
	pool.StopAndWait()

	app.logger.Info("Finished archiving old stats data")
	return nil
}

func (app app) archiveStory(ctx context.Context, sc *StorageClient, storyID int) error {

	app.logger.Debug("Skipping archiving of story for now", "storyID", storyID)
	return nil

	filename := fmt.Sprintf("%d.json", storyID)

	// Check if the file already exists before uploading
	exists, err := sc.FileExists(ctx, filename)
	if err != nil {
		return errors.Wrapf(err, "checking if file %s exists", filename)
	}

	if exists {
		app.logger.Info("File already archived", "filename", filename)
	} else {
		jsonData, err := app.generateStatsDataJSON(ctx, storyID)
		if err != nil {
			return errors.Wrapf(err, "generating stats data for story %d", storyID)
		}

		app.logger.Debug("Uploading archive file", "storyID", storyID)
		err = sc.UploadFile(ctx, filename, jsonData, "application/json", true)
		if err != nil {
			return errors.Wrapf(err, "uploading file %s", filename)
		}
	}

	app.logger.Debug("Deleting old statsData", "storyID", storyID)
	n, err := app.ndb.deleteOldData(storyID)
	if err != nil {
		return errors.Wrapf(err, "deleting %d rows of data for story %d", n, storyID)
	}

	app.logger.Info("Archived stats data for story", "rowsDeleted", n, "storyID", storyID)
	return nil
}
