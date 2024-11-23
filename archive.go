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

func (app app) archiveOldStatsData(ctx context.Context) error {
	app.logger.Debug("selectStoriesToArchive")
	storyIDs, err := app.ndb.selectStoriesToArchive(ctx)
	if err != nil {
		return errors.Wrap(err, "selectStoriesToArchive")
	}
	app.logger.Debug("Finished selectStoriesToArchive", "nStories", len(storyIDs))

	if len(storyIDs) == 0 {
		return nil // Nothing to archive
	}

	sc, err := NewStorageClient()
	if err != nil {
		return errors.Wrap(err, "create storage client")
	}

	app.logger.Debug("Uploading archive JSON files")
	for _, storyID := range storyIDs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		filename := fmt.Sprintf("%d.json", storyID)

		// Check if the file already exists before uploading
		exists, err := sc.FileExists(ctx, filename)
		if err != nil {
			app.logger.Error("checking if file exists", err, "filename", filename)
			continue // Continue with the next storyID
		}

		if exists {
			app.logger.Info("File already archived", "filename", filename)
		} else {
			jsonData, err := app.generateStatsDataJSON(ctx, storyID)
			if err != nil {
				app.logger.Error("generating stats data for story", err, "storyID", storyID)
				continue // Continue with the next storyID
			}

			app.logger.Debug("Uploading archive file", "storyID", storyID)
			err = sc.UploadFile(ctx, filename, jsonData, "application/json", true)
			if err != nil {
				app.logger.Error(fmt.Sprintf("uploading file %s", filename), err)
				continue // Continue with the next storyID
			}
		}

		app.logger.Debug("Deleting old statsData", "storyID", storyID)
		n, err := app.ndb.deleteOldData(storyID)
		if err != nil {
			app.logger.Error("deleting old data for story", err, "rowsDeleted", n, "storyID", storyID)
			continue // Continue with the next storyID
		}

		app.logger.Info("Archived stats data for story", "rowsDeleted", n, "storyID", storyID)
	}

	app.logger.Info("Finished archiving old stats data")
	return nil
}
