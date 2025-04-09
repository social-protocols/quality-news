package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

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
	maxSampleTime, err := maxSampleTime(ctx, ndb, storyID)
	if err != nil {
		return nil, errors.Wrap(err, "maxSampleTime")
	}

	// Fetch RanksPlotData
	ranksPlotData, err := rankDatapoints(ctx, ndb, storyID)
	if err != nil {
		return nil, errors.Wrap(err, "rankDatapoints")
	}

	// Fetch UpvotesPlotData
	upvotesPlotData, err := upvotesDatapoints(ctx, ndb, storyID, modelParams)
	if err != nil {
		return nil, errors.Wrap(err, "upvotesDatapoints")
	}

	// Fetch Story details
	s, err := ndb.selectStoryDetails(ctx, storyID)
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
	storyID int
	err     error
}

func (app app) uploadStoryArchive(ctx context.Context, sc *StorageClient, storyID int) archiveResult {
	// Version 2 includes full story details and allows deletion of story record

	app.logger.Debug("uploadStoryArchive", "storyID", storyID)

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
		return archiveResult{storyID: storyID}
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

	app.logger.Info("Uploading archive file", "storyID", storyID)
	err = sc.UploadFile(ctx, filename, jsonData, "application/json", true)
	if err != nil {
		return archiveResult{storyID: storyID, err: errors.Wrapf(err, "uploading file %s", filename)}
	}

	// Check if context was cancelled during/after upload
	if err := ctx.Err(); err != nil {
		return archiveResult{storyID: storyID, err: errors.Wrap(err, "context cancelled after upload")}
	}

	return archiveResult{storyID: storyID}
}


// processArchivingOperations handles a batch of archiving operations using a worker pool.
// It selects stories eligible for archiving, then processes
// each story in parallel using the provided worker pool.
//
// The function handles errors at both the batch and individual story level, ensuring
// that failures in one story don't prevent others from being processed.
func (app app) processArchivingOperations(ctx context.Context) {
	logger := app.logger

	// Get stories to archive
	storyIDs, err := app.ndb.selectStoriesToArchive(ctx)
	if err != nil {
		archiveErrorsTotal.Inc()
		logger.Error("Failed to select stories for archiving", err)
		return
	}

	if len(storyIDs) == 0 {
		return
	}

	logger.Info("Found stories to archive", "count", len(storyIDs))

	// Create storage client
	sc, err := NewStorageClient()
	if err != nil {
		archiveErrorsTotal.Inc()
		logger.Error("Failed to create storage client", err)
		return
	}

	results := make(chan archiveResult, len(storyIDs))
	defer close(results)

	pool := pond.NewPool(10, pond.WithContext(ctx))

	var archived int
	var uploadErrors int
	var wg sync.WaitGroup
	wg.Add(1)

	// Start goroutine to process results
	go func() {
		defer wg.Done()
		for i := 0; i < len(storyIDs); i++ {
			result := <-results
			if result.err != nil {
				uploadErrors++
				archiveErrorsTotal.Inc()
				logger.Error("Failed to archive story", result.err, "storyID", result.storyID)
				continue
			}
			archived++

			// Mark story as archived in database
			if err := app.ndb.markStoryArchived(ctx, result.storyID); err != nil {
				archiveErrorsTotal.Inc()
				logger.Error("Failed to mark story as archived", err, "storyID", result.storyID)
				continue
			}

			storiesArchivedTotal.Inc()
		}
	}()

	// Submit all work to the pool
	for _, storyID := range storyIDs {
		sid := storyID
		pool.Submit(func() {

			// Perform the upload
            if err := ctx.Err(); err != nil {
                archiveErrorsTotal.Inc()
                results <- archiveResult{storyID: sid, err: errors.Wrap(err, "context cancelled before starting upload")}
                return
            }
			results <- app.uploadStoryArchive(ctx, sc, sid)
		})
	}

	pool.StopAndWait()
	wg.Wait()

    app.logger.Info("Finished archiving",
	    "found", len(storyIDs),
	    "archived", archived,
	    "archive_errors", uploadErrors,
	)
}


// archiveWorker runs in a separate goroutine and handles both archiving and purging operations.
// It processes archiving tasks continuously using a worker pool, and handles purging operations
// during idle periods signaled by the main loop.
//
// The worker maintains two main operations:
// 1. Continuous archiving: Runs every 5 minutes to archive eligible stories
// 2. Triggered purging: Runs during idle periods between crawls to purge archived data
//
// The worker respects context cancellation and properly handles task timeouts.
func (app app) archiveWorker(ctx context.Context) {
	logger := app.logger

	app.processArchivingOperations(ctx)

	// Create a ticker for periodic archiving
	archiveTicker := time.NewTicker(5 * time.Minute)
	defer archiveTicker.Stop()

	for {
		select {
		case idleCtx := <-app.archiveTriggerChan:
			app.processPurgeOperations(idleCtx)

		case <-archiveTicker.C:
			app.processArchivingOperations(ctx)

		case <-ctx.Done():
			logger.Info("Archive worker shutting down")
			return
		}
	}
}

// processPurgeOperations handles purge operations during an idle period.
// It attempts to purge as many archived stories as possible before the context is cancelled.
// If no stories are found to purge, it attempts to delete old generic data.
//
// The operation respects the provided context's deadline and properly handles
// cancellation and timeout scenarios.
func (app app) processPurgeOperations(ctx context.Context) {
	logger := app.logger
	var purgedCount int

	// Keep processing purge operations until context is cancelled
	// or until nothing is to be done.
	for {
		// Try to purge one story first
		storyID, err := app.ndb.selectStoryToPurge(ctx)
		if err != nil {
			logger.Error("Failed to select story for purging", err)
			return
		}

		if storyID != 0 {
			// Found a story to purge
			if err := app.ndb.purgeStory(ctx, storyID); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					logger.Info("Purge operation cancelled due to deadline", "storyID", storyID)
					return
				}
				logger.Error("Failed to purge story", err, "storyID", storyID)
				// Continue to next story on error
				continue
			}
			purgedCount++
			logger.Info("Successfully purged story", "storyID", storyID, "totalPurged", purgedCount)
			continue
		}

		// If no story to purge, try to delete old data
		rowsDeleted, err := app.ndb.deleteOldData(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				logger.Info("Delete old data operation cancelled due to deadline")
				return
			}
			logger.Error("Failed to delete old data", err)
		}

		if rowsDeleted > 0 {
			logger.Info("Deleted old data", "rowsDeleted", rowsDeleted)
		}

		return
	}
}
