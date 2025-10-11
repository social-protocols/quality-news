package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
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

// processArchivingOperations handles old story cleanup by either archiving or marking for deletion.
// It operates in three phases:
//  1. Selects stories older than 21 days that haven't been processed yet
//  2. For high-score stories (score > 2): generates JSON and uploads to S3 for backup
//     For low-score stories (score ≤ 2): skips S3 upload (no backup needed)
//  3. Marks all processed stories as archived (ready for deletion by purge worker)
//
// The function uses goroutine pools to parallelize the work, with a default
// concurrency of 10 workers. Results are collected via a buffered channel, and errors
// are logged but don't stop the processing of other stories.
//
// This approach ensures valuable high-score stories are preserved in S3 while allowing
// low-score stories to be cleaned up without the cost of backup storage.
//
// The operation has a 4 minute and 30 second timeout to ensure it completes before
// the next scheduled run.
func (app app) processArchivingOperations(ctx context.Context) error {
	logger := app.logger

	// Create a timeout context for the entire operation
	timeoutCtx, cancel := context.WithTimeout(ctx, 4*time.Minute+30*time.Second)
	defer cancel()

	logger.Info("Selecting stories to process (stories older than 21 days not yet archived)")

	// Get stories to archive
	storyIDs, err := app.ndb.selectStoriesToArchive(timeoutCtx)
	if err != nil {
		archiveErrorsTotal.Inc()
		logger.Error("Failed to select stories for archiving", err)
		return err
	}

	if len(storyIDs) == 0 {
		logger.Info("No old stories found to process")
	} else {
		logger.Info("Found old stories to process", "count", len(storyIDs), "story_ids", storyIDs)
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
			// Recover from panics in worker tasks
			defer func() {
				if r := recover(); r != nil {
					archiveErrorsTotal.Inc()
					logger.Error("Archive task panic", fmt.Errorf("panic in story %d: %v", sid, r), "storyID", sid)
					results <- archiveResult{storyID: sid, err: fmt.Errorf("panic: %v", r)}
				}
			}()

			// Check context
			if err := timeoutCtx.Err(); err != nil {
				archiveErrorsTotal.Inc()
				results <- archiveResult{storyID: sid, err: errors.Wrap(err, "context cancelled")}
				return
			}

			// Get max score to decide whether to upload to S3
			maxScore, err := app.ndb.getMaxScore(timeoutCtx, sid)
			if err != nil {
				archiveErrorsTotal.Inc()
				results <- archiveResult{storyID: sid, err: errors.Wrap(err, "failed to get max score")}
				return
			}

			if maxScore > 2 {
				// High-score story: upload to S3 for backup
				logger.Debug("Archiving story to S3", "storyID", sid, "maxScore", maxScore)
				results <- app.uploadStoryArchive(timeoutCtx, sc, sid)
			} else {
				// Low-score story: skip S3 upload, just mark for deletion
				logger.Debug("Marking low-score story for deletion (no S3 backup)", "storyID", sid, "maxScore", maxScore)
				results <- archiveResult{storyID: sid, err: nil}
			}
		})
	}

	pool.StopAndWait()
	wg.Wait()

	app.logger.Info("Finished processing old stories",
		"found", len(storyIDs),
		"marked_for_deletion", archived,
		"errors", uploadErrors,
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

	// Recover from panics to prevent worker from dying
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Archive worker panic recovered", fmt.Errorf("panic: %v", r))
			// Worker will exit but at least we'll know why
		}
	}()

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

	// Recover from panics to prevent worker from dying
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Purge worker panic recovered", fmt.Errorf("panic: %v", r))
			// Worker will exit but at least we'll know why
		}
	}()

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

	// Log database stats for monitoring
	// Weekly VACUUM (Sunday 3 AM) handles fragmentation
	logger.Debug("Getting DB stats")
	size, freelist, fragmentation, err := app.ndb.getDatabaseStats()
	if err != nil {
		logger.Error("Failed to get database stats", err)
	} else {
		logger.Debug("Database stats",
			"size_mb", float64(size)/(1024*1024),
			"freelist_pages", freelist,
			"fragmentation_pct", fragmentation)
	}

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

// vacuumWorker runs in a separate goroutine and performs weekly database compaction.
// It runs on Sunday mornings at 3 AM to minimize impact on users.
//
// The worker uses VACUUM INTO to create a compacted copy of the database without
// blocking read operations. Once complete, it swaps the files during a brief pause.
//
// This prevents database file size from growing indefinitely due to fragmentation
// from the continuous cycle of deleting old data and adding new data.
func (app app) vacuumWorker(ctx context.Context) {
	logger := app.logger

	// Recover from panics to prevent worker from dying
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Vacuum worker panic recovered", fmt.Errorf("panic: %v", r))
		}
	}()

	logger.Info("Vacuum worker started - will run Sundays at 3 AM")

	// Check every hour if it's time to vacuum
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()

			// Only run on Sunday between 3 AM and 4 AM
			if now.Weekday() != time.Sunday {
				continue
			}
			if now.Hour() != 3 {
				continue
			}

			logger.Info("Starting weekly database VACUUM")

			// Check if vacuum is needed
			_, freelist, fragmentation, err := app.ndb.getDatabaseStats()
			if err != nil {
				logger.Error("Failed to get database stats", err)
				continue
			}

			logger.Info("Database stats before VACUUM",
				"fragmentation_pct", fragmentation,
				"freelist_pages", freelist)

			if fragmentation < 15.0 {
				logger.Info("Fragmentation low - skipping VACUUM",
					"fragmentation_pct", fragmentation)
				continue
			}

			// Perform VACUUM INTO (creates compacted copy)
			err = app.performWeeklyVacuum(ctx)
			if err != nil {
				logger.Error("Weekly VACUUM failed", err)
				continue
			}

			logger.Info("Weekly VACUUM completed successfully")

			// Sleep for remainder of hour to avoid running multiple times
			time.Sleep(55 * time.Minute)

		case <-ctx.Done():
			logger.Info("Vacuum worker shutting down")
			return
		}
	}
}

// performWeeklyVacuum creates a compacted copy of the database and swaps it in
func (app app) performWeeklyVacuum(ctx context.Context) error {
	logger := app.logger
	ndb := app.ndb

	// Create compacted copy using VACUUM INTO
	newDBPath := fmt.Sprintf("%s/frontpage_new.sqlite", ndb.sqliteDataDir)
	oldDBPath := fmt.Sprintf("%s/frontpage.sqlite", ndb.sqliteDataDir)
	backupPath := fmt.Sprintf("%s/frontpage_backup_%s.sqlite",
		ndb.sqliteDataDir,
		time.Now().Format("2006_01_02"))

	logger.Info("Creating compacted database copy", "target", newDBPath)

	// Use VACUUM INTO to create compacted copy (doesn't block reads)
	_, err := ndb.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", newDBPath))
	if err != nil {
		return errors.Wrap(err, "VACUUM INTO failed")
	}

	logger.Info("Compacted database created successfully")
	logger.Info("Swapping database files - brief service interruption expected")

	// Close current connection
	err = ndb.db.Close()
	if err != nil {
		return errors.Wrap(err, "failed to close database")
	}

	// Rename old database as backup
	err = os.Rename(oldDBPath, backupPath)
	if err != nil {
		return errors.Wrap(err, "failed to backup old database")
	}

	// Move new database into place
	err = os.Rename(newDBPath, oldDBPath)
	if err != nil {
		// Try to restore backup
		os.Rename(backupPath, oldDBPath)
		return errors.Wrap(err, "failed to move new database")
	}

	// Reconnect to new database
	logger.Info("Reconnecting to compacted database")
	newDB, err := sql.Open("sqlite3_ext",
		fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000", oldDBPath))
	if err != nil {
		return errors.Wrap(err, "failed to reconnect to database")
	}

	ndb.db = newDB

	// Get new stats
	_, freelist, fragmentation, err := ndb.getDatabaseStats()
	if err != nil {
		logger.Error("Failed to get stats after VACUUM", err)
	} else {
		logger.Info("Database stats after VACUUM",
			"fragmentation_pct", fragmentation,
			"freelist_pages", freelist)
	}

	logger.Info("Database swap complete",
		"old_backup", backupPath,
		"note", "Old database kept as backup for 24 hours")

	return nil
}
