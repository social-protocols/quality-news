package main

import (
	"context"
	"encoding/json"
	"fmt"
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

func (app app) archiveOldStatsData(ctx context.Context) ([]int, error) {
	storyIDs, err := app.ndb.selectStoriesToArchive()
	if err != nil {
		return nil, errors.Wrap(err, "selectStoriesToArchive")
	}

	if len(storyIDs) == 0 {
		return nil, nil // Nothing to archive
	}

	sc, err := NewStorageClient()
	if err != nil {
		return nil, errors.Wrap(err, "create storage client")
	}

	var archivedStoryIDs []int

	for _, storyID := range storyIDs {
		select {
		case <-ctx.Done():
			return archivedStoryIDs, ctx.Err()
		default:
		}

		filename := fmt.Sprintf("%d.json", storyID)

		// Check if the file already exists before uploading
		exists, err := sc.FileExists(ctx, filename)
		if err != nil {
			app.logger.Error(fmt.Sprintf("checking if file %s exists", filename), err)
			continue // Continue with the next storyID
		}
		if exists {
			app.logger.Debug("File already archived", "filename", filename)
			archivedStoryIDs = append(archivedStoryIDs, storyID)
			continue // Skip uploading if the file is already archived
		}

		jsonData, err := app.generateStatsDataJSON(ctx, storyID)
		if err != nil {
			app.logger.Error(fmt.Sprintf("generating stats data for storyId %d", storyID), err)
			continue // Continue with the next storyID
		}

		err = sc.UploadFile(ctx, filename, jsonData, "application/json", true)
		if err != nil {
			app.logger.Error(fmt.Sprintf("uploading file %s", filename), err)
			continue // Continue with the next storyID
		}

		app.logger.Debug("Archived stats data for storyID", "storyID", storyID)
		archivedStoryIDs = append(archivedStoryIDs, storyID)
	}

	return archivedStoryIDs, nil
}

func (app app) runArchivingTasks(ctx context.Context) error {
	archivedStoryIDs, err := app.archiveOldStatsData(ctx)
	if err != nil {
		return errors.Wrap(err, "archiveOldStatsData")
	}

	nStories := len(archivedStoryIDs)
	var nDeleted int64 = 0

	// loop through storeis and delete one by one
	for _, storyID := range archivedStoryIDs {
		n, err := app.ndb.deleteOldData(storyID)
		if err != nil {
			app.logger.Error(fmt.Sprintf("deleting old data for storyId %d", storyID), err)
		} else {
			nDeleted += n
		}
	}

	app.logger.Info(fmt.Sprintf("Deleted %d rows from DB for %d stories", nDeleted, nStories))

	return nil
}
