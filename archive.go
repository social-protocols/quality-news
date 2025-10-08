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

	logger.Info("Selecting stories to archive (stories older than 21 days with score > 2 and not yet archived)")

	// Get stories to archive
	storyIDs, err := app.ndb.selectStoriesToArchive(timeoutCtx)
	if err != nil {
		archiveErrorsTotal.Inc()
		logger.Error("Failed to select stories for archiving", err)
		return err
	}

	if len(storyIDs) == 0 {
		logger.Info("No stories found that need archiving")
	} else {
		logger.Info("Found stories to archive", "count", len(storyIDs), "story_ids", storyIDs)
	}

	if len(storyIDs) == 0 {
		logger.Debug("Archiving cycle complete - no work to do")
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

// archiveWorker runs in a separate goroutine and handles archiving operations.
// It processes archiving tasks on a 5-minute schedule with a 30-second offset from the minute mark.
//
// This worker runs independently of the purge worker, so long-running archiving operations
// do not block purge operations from receiving idle signals.
//
// The worker respects context cancellation and properly handles task timeouts.
func (app app) archiveWorker(ctx context.Context) {
	logger := app.logger

	logger.Info("Archive worker started")

	// Calculate initial delay until next 1-minute mark + 30 seconds
	now := time.Now()
	nextRun := now.Truncate(1 * time.Minute).Add(30 * time.Second)
	initialDelay := nextRun.Sub(now)

	logger.Debug("Archive worker waiting for initial delay", "delay_seconds", initialDelay.Seconds())

	// Create a ticker for periodic archiving
	archiveTicker := time.NewTicker(5 * time.Minute)
	defer archiveTicker.Stop()

	// Use a timer for the initial delay so we can select on it
	initialTimer := time.NewTimer(initialDelay)
	defer initialTimer.Stop()

	// Wait for initial delay in the select loop
	initialRun := true

	for {
		select {
		case <-initialTimer.C:
			if initialRun {
				initialRun = false
				logger.Info("Running initial archiving operation")
				if err := app.processArchivingOperations(ctx); err != nil {
					logger.Error("Initial archiving operation failed", err)
				}
			}

		case <-archiveTicker.C:
			logger.Info("Running scheduled archiving operation")
			if err := app.processArchivingOperations(ctx); err != nil {
				logger.Error("Scheduled archiving operation failed", err)
				// Continue running even if archiving fails - don't crash the worker
			}

		case <-ctx.Done():
			logger.Info("Archive worker shutting down")
			return
		}
	}
}

// purgeWorker runs in a separate goroutine and handles purge operations during idle periods.
// It listens for idle signals from the main loop (sent after each crawl completes) and
// performs purge operations during the remaining time before the next crawl.
//
// This worker runs independently of the archive worker, ensuring that long-running
// archiving operations do not prevent purge operations from executing.
//
// The worker respects context cancellation and properly handles operation timeouts.
func (app app) purgeWorker(ctx context.Context) {
	logger := app.logger

	logger.Info("Purge worker started")

	for {
		select {
		case idleCtx := <-app.archiveTriggerChan:
			// Create a timeout context to ensure we don't block too long
			// Give ourselves a reasonable amount of time but ensure we're done before next crawl
			timeoutCtx, cancel := context.WithTimeout(idleCtx, 50*time.Second)

			if err := app.processPurgeOperations(timeoutCtx); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					logger.Info("Purge operation timed out - will continue next cycle")
				} else {
					logger.Error("Purge operation failed", err)
				}
			}
			cancel() // Clean up the timeout context

		case <-ctx.Done():
			logger.Info("Purge worker shutting down")
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

	// If no stories to purge, try to perform incremental vacuum
	// Use a reasonable number of pages (e.g., 1000) to avoid long operations
	logger.Debug("Getting DB stats")
	size, freelist, fragmentation, err := app.ndb.getDatabaseStats()
	if err != nil {
		return errors.Wrap(err, "getDatabaseStats")
	}
	logger.Info("Database stats",
		"size_mb", float64(size)/(1024*1024),
		"freelist_pages", freelist,
		"fragmentation_pct", fragmentation)

	const maxVacuumPages = 1000
	if err := app.ndb.incrementalVacuum(ctx, maxVacuumPages); err != nil {
		logger.Error("Failed to perform incremental vacuum", err)
		// Don't return the error - vacuuming is optional
	}
	logger.Debug("Finished vacuum")

	var purgedCount int
	var totalRowsPurged int64

	// Count stories needing purge
	storiesNeedingPurge, err := app.ndb.countStoriesNeedingPurge(ctx)
	if err != nil {
		logger.Error("Failed to count stories needing purge", err)
		return err
	}
	logger.Info("Starting purge operations", "storiesNeedingPurge", storiesNeedingPurge)

	// Keep processing purge operations until context is cancelled
	// or until nothing is to be done.
	for {
		// Check if context is cancelled before each iteration
		select {
		case <-ctx.Done():
			logger.Info("Purge operation cancelled by context",
				"storiesPurged", purgedCount,
				"totalRowsPurged", totalRowsPurged)
			return nil
		default:
		}

		// Try to purge one story first
		logger.Info("Selecting story to purge")
		storyID, err := app.ndb.selectStoryToPurge(ctx)
		if err != nil {
			logger.Error("Failed to select story for purging", err)
			return err
		}

		if storyID != 0 {
			// Found a story to purge
			logger.Info("Purging story", "storyID", storyID)
			rowsPurged, err := app.ndb.purgeStory(ctx, storyID)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					logger.Info("Purge operation cancelled due to deadline",
						"storyID", storyID,
						"storiesPurged", purgedCount,
						"totalRowsPurged", totalRowsPurged)
					return nil
				}
				logger.Error("Failed to purge story", err, "storyID", storyID)
				// Continue to next story on error
				continue
			}
			purgedCount++
			totalRowsPurged += rowsPurged
			storiesPurgedTotal.Inc()
			logger.Info("Successfully purged story",
				"storyID", storyID,
				"rowsPurged", rowsPurged,
				"storiesPurged", purgedCount,
				"totalRowsPurged", totalRowsPurged)
			continue
		} else {
			logger.Info("No stories to purge")
			break
		}
	}

	const deleteOldData = false

	// If no story to purge, try to delete old data
	if deleteOldData {
		// Check if we still have time for old data deletion
		select {
		case <-ctx.Done():
			logger.Info("Skipping old data deletion - context cancelled",
				"storiesPurged", purgedCount,
				"totalRowsPurged", totalRowsPurged)
			return nil
		default:
		}

		logger.Info("Deleting old data")
		rowsDeleted, err := app.ndb.deleteOldData(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				logger.Info("Delete old data operation cancelled due to deadline", "rowsDeleted", rowsDeleted)
				return nil
			}
			logger.Error("Failed to delete old data", err)
		}
		logger.Info("Deleted old data",
			"rowsDeleted", rowsDeleted)
	}

	return nil
}
