package main

import (
	"context"
	"encoding/json"
	"fmt"
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

	app.logger.Debug("generateArchiveJSON", "storyID", storyID)
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
//
// The operation has a 4 minute and 30 second timeout to ensure it completes before
// the next scheduled run.
func (app app) processArchivingOperations(ctx context.Context) error {
	logger := app.logger

	// Create a timeout context for the entire operation
	timeoutCtx, cancel := context.WithTimeout(ctx, 4*time.Minute+30*time.Second)
	defer cancel()

	logger.Info("Selecting stories to archive")

	// Get stories to archive
	storyIDs, err := app.ndb.selectStoriesToArchive(timeoutCtx)
	if err != nil {
		archiveErrorsTotal.Inc()
		logger.Error("Failed to select stories for archiving", err)
		return err
	}

	logger.Info("Found stories to archive", "count", len(storyIDs))

	if len(storyIDs) == 0 {
		return nil
	}

	// Create storage client
	sc, err := NewStorageClient()
	if err != nil {
		archiveErrorsTotal.Inc()
		logger.Error("Failed to create storage client", err)
		return err
	}

	results := make(chan archiveResult, len(storyIDs))
	defer close(results)

	pool := pond.NewPool(10, pond.WithContext(timeoutCtx))

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
			logger.Debug("Marking story as archived", "storyID", result.storyID)
			if err := app.ndb.markStoryArchived(timeoutCtx, result.storyID); err != nil {
				archiveErrorsTotal.Inc()
				logger.Error("Failed to mark story as archived", err, "storyID", result.storyID)
				continue
			}
			logger.Debug("Marked story as archived", "storyID", result.storyID)

			storiesArchivedTotal.Inc()
		}
	}()

	// Submit all work to the pool
	for _, storyID := range storyIDs {
		sid := storyID
		pool.Submit(func() {
			// Perform the upload
			if err := timeoutCtx.Err(); err != nil {
				archiveErrorsTotal.Inc()
				results <- archiveResult{storyID: sid, err: errors.Wrap(err, "context cancelled before starting upload")}
				return
			}
			results <- app.uploadStoryArchive(timeoutCtx, sc, sid)
		})
	}

	pool.StopAndWait()
	wg.Wait()

	app.logger.Info("Finished archiving",
		"found", len(storyIDs),
		"archived", archived,
		"archive_errors", uploadErrors,
	)

	return nil
}

// archiveWorker runs in a separate goroutine and handles both archiving and purging operations.
// It processes archiving tasks on a 5-minute schedule with a 30-second offset from the minute mark,
// and handles purging operations during idle periods signaled by the main loop.
//
// The worker maintains two main operations:
// 1. Scheduled archiving: Runs every 5 minutes at :30 seconds past the minute
// 2. Triggered purging: Runs during idle periods between crawls to purge archived data
//
// The worker respects context cancellation and properly handles task timeouts.
// If the worker encounters a fatal error, it will log the error and restart itself.
func (app app) archiveWorker(ctx context.Context) {
	logger := app.logger

	// Calculate initial delay until next 1-minute mark + 30 seconds
	now := time.Now()
	nextRun := now.Truncate(1 * time.Minute).Add(30 * time.Second)
	initialDelay := nextRun.Sub(now)

	<-time.After(initialDelay)

	// Create a ticker for periodic archiving
	archiveTicker := time.NewTicker(5 * time.Minute)
	defer archiveTicker.Stop()

	// Run initial archiving after delay
	if err := app.processArchivingOperations(ctx); err != nil {
		logger.Error("Initial archiving operation failed", err)
	}

	for {
		select {
		case idleCtx := <-app.archiveTriggerChan:
			if err := app.processPurgeOperations(idleCtx); err != nil {
				logger.Error("Purge operation failed", err)
			}

		case <-archiveTicker.C:
			if err := app.processArchivingOperations(ctx); err != nil {
				logger.Error("Scheduled archiving operation failed", err)
				// If archiving fails, we should restart the worker
				// This will be handled by the main loop
				return
			}

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
func (app app) processPurgeOperations(ctx context.Context) error {
	logger := app.logger
	var purgedCount int

	// Keep processing purge operations until context is cancelled
	// or until nothing is to be done.
	for {
		// Try to purge one story first
		logger.Info("Selecting stories to purge")
		storyID, err := app.ndb.selectStoryToPurge(ctx)
		if err != nil {
			logger.Error("Failed to select story for purging", err)
			return err
		}

		if storyID != 0 {
			// Found a story to purge
			logger.Info("Purging story", "storyID", storyID)
			if err := app.ndb.purgeStory(ctx, storyID); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					logger.Info("Purge operation cancelled due to deadline", "storyID", storyID, "storiesPurged", purgedCount)
					return nil
				}
				logger.Error("Failed to purge story", err, "storyID", storyID)
				// Continue to next story on error
				continue
			}
			purgedCount++
			storiesPurgedTotal.Inc()
			logger.Info("Successfully purged story", "storyID", storyID, "totalPurged", purgedCount)
			continue
		}

		// If no story to purge, try to delete old data
		rowsDeleted, err := app.ndb.deleteOldData(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				logger.Info("Delete old data operation cancelled due to deadline")
				return nil
			}
			logger.Error("Failed to delete old data", err)
		}

		if rowsDeleted > 0 {
			logger.Info("Deleted old data", "rowsDeleted", rowsDeleted)
		}
		return nil
	}
}
