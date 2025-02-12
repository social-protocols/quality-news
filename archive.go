package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/exp/slog"

	pond "github.com/alitto/pond/v2"
	"github.com/pkg/errors"
)

type ArchiveData struct {
	RanksPlotData   [][]any `json:"RanksPlotData"`
	UpvotesPlotData [][]any `json:"UpvotesPlotData"`
	MaxSampleTime   int     `json:"MaxSampleTime"`
	Story                   // embed Story
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

func (app app) generateArchiveJSON(ctx context.Context, storyID int) ([]byte, error) {
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

	// Fetch Story details
	s, err := ndb.selectStoryDetails(storyID)
	if err != nil {
		return nil, errors.Wrap(err, "selectStoryDetails")
	}

	// Create ArchiveData struct with story details
	archiveData := ArchiveData{
		RanksPlotData:   ranksPlotData,
		UpvotesPlotData: upvotesPlotData,
		MaxSampleTime:   maxSampleTime,
		Story:           s,
	}

	jsonData, err := json.Marshal(archiveData)
	if err != nil {
		return nil, errors.Wrap(err, "json.Marshal archiveData")
	}

	return jsonData, nil
}

type archiveResult struct {
	storyID  int
	uploaded bool
	err      error
}

func (app app) uploadStoryArchive(ctx context.Context, sc *StorageClient, storyID int) archiveResult {
	// Version 2 includes full story details and allows deletion of story record
	const archiveVersion = 2
	filename := fmt.Sprintf("%d.v%d.json", storyID, archiveVersion)

	// Check for any version of the file
	legacyExists, err := sc.FileExists(ctx, fmt.Sprintf("%d.json", storyID))
	if err != nil {
		return archiveResult{storyID: storyID, err: errors.Wrapf(err, "checking if legacy file exists")}
	}

	exists, err := sc.FileExists(ctx, filename)
	if err != nil {
		return archiveResult{storyID: storyID, err: errors.Wrapf(err, "checking if file %s exists", filename)}
	}

	if exists {
		app.logger.Info("File already archived", "filename", filename)
		return archiveResult{storyID: storyID, uploaded: true}
	}

	if legacyExists {
		app.logger.Warn("Legacy archive already exists", "storyID", storyID)
		err = sc.DeleteFile(ctx, fmt.Sprintf("%d.json", storyID))
		if err != nil {
			return archiveResult{storyID: storyID, err: errors.Wrapf(err, "deleting legacy archive file")}
		}
	}

	jsonData, err := app.generateArchiveJSON(ctx, storyID)
	if err != nil {
		return archiveResult{storyID: storyID, err: errors.Wrapf(err, "generating archive data for story %d", storyID)}
	}

	app.logger.Debug("Uploading archive file", "storyID", storyID)
	err = sc.UploadFile(ctx, filename, jsonData, "application/json", true)
	if err != nil {
		return archiveResult{storyID: storyID, err: errors.Wrapf(err, "uploading file %s", filename)}
	}

	return archiveResult{storyID: storyID, uploaded: true}
}

func (app app) archiveAndPurgeOldStatsData(ctx context.Context) error {
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

		var purged int

		// Now purge all successfully archived stories
		app.logger.Info("Purging archived stories", "count", len(successfulUploads))
		for _, storyID := range successfulUploads {
			// Check if context was cancelled before each purge
			if err := ctx.Err(); err != nil {
				app.logger.Error("Context cancelled during purge", err,
					"stories_purged", purged,
					"stories_remaining", len(successfulUploads)-purged)
				return errors.Wrap(err, "context cancelled during purge")
			}

			err := app.ndb.purgeStory(ctx, storyID)
			if err != nil {
				app.logger.Error("Failed to purge archived story", err,
					"storyID", storyID)
				continue
			}
			purged++
		}

		app.logger.Info("Finished archiving",
			"archived", len(successfulUploads),
			"purged_archived", purged)
	} else {
		app.logger.Info("No stories to archive")
	}

	// Delete old data
	app.logger.Info("Deleting old data")
	rowsDeleted, err := app.ndb.deleteOldData(ctx)
	if err != nil {
		return errors.Wrap(err, "deleteOldData")
	}
	if rowsDeleted > 0 {
		app.logger.Info("Deleted old data", slog.Int64("rows_deleted", rowsDeleted))
	}

	return nil
}
