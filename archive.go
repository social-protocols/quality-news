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

type archiveResult struct {
	storyID  int
	uploaded bool
	err      error
}

func (app app) uploadStoryArchive(ctx context.Context, sc *StorageClient, storyID int) archiveResult {
	filename := fmt.Sprintf("%d.json", storyID)

	exists, err := sc.FileExists(ctx, filename)
	if err != nil {
		return archiveResult{storyID: storyID, err: errors.Wrapf(err, "checking if file %s exists", filename)}
	}

	if exists {
		app.logger.Info("File already archived", "filename", filename)
		return archiveResult{storyID: storyID, uploaded: true}
	}

	jsonData, err := app.generateStatsDataJSON(ctx, storyID)
	if err != nil {
		return archiveResult{storyID: storyID, err: errors.Wrapf(err, "generating stats data for story %d", storyID)}
	}

	app.logger.Debug("Uploading archive file", "storyID", storyID)
	err = sc.UploadFile(ctx, filename, jsonData, "application/json", true)
	if err != nil {
		return archiveResult{storyID: storyID, err: errors.Wrapf(err, "uploading file %s", filename)}
	}

	return archiveResult{storyID: storyID, uploaded: true}
}

func (app app) archiveAndPurgeOldStatsData(ctx context.Context) error {

	app.logger.Info("Selecting stories to purge")
	storyIDsToPurge, err := app.ndb.selectStoriesToPurge(ctx)
	if err != nil {
		return errors.Wrap(err, "selectStoriesToPurge")
	}

	for _, storyID := range storyIDsToPurge {
		err := app.ndb.purgeStory(storyID)
		if err != nil {
			app.logger.Error("Failed to purge story", err,
				"storyID", storyID)
			continue
		}
	}
	app.logger.Info("Purged", "purged", len(storyIDsToPurge))

	app.logger.Info("Looking for stories to archive")
	storyIDsToArchive, err := app.ndb.selectStoriesToArchive(ctx)
	if err != nil {
		return errors.Wrap(err, "selectStoriesToArchive")
	}

	if len(storyIDsToArchive) > 0 {
		app.logger.Info("Found stories to archive", "count", len(storyIDsToArchive))
		sc, err := NewStorageClient()
		if err != nil {
			return errors.Wrap(err, "create storage client")
		}

		results := make(chan archiveResult, len(storyIDsToArchive))
		pool := pond.NewPool(10, pond.WithContext(ctx))

		for _, storyID := range storyIDsToArchive {
			sid := storyID
			pool.Submit(func() {
				result := app.uploadStoryArchive(ctx, sc, sid)
				results <- result
			})
		}

		go func() {
			pool.StopAndWait()
			close(results)
		}()

		successfulUploads := make([]int, 0, len(storyIDsToArchive))
		for result := range results {
			if result.err != nil {
				app.logger.Error("Failed to archive story", result.err,
					"storyID", result.storyID)
				continue
			}
			if result.uploaded {
				successfulUploads = append(successfulUploads, result.storyID)
			}
		}

		for _, storyID := range successfulUploads {
			n, err := app.ndb.deleteOldData(storyID)
			if err != nil {
				app.logger.Error("Failed to delete old data", err,
					"storyID", storyID)
				continue
			}
			app.logger.Info("Archived stats data for story",
				"rowsDeleted", n,
				"storyID", storyID)
		}
	}

	app.logger.Info("Finished archive and purge",
		"purged", len(storyIDsToPurge),
		"archived", len(storyIDsToArchive))
	return nil
}
