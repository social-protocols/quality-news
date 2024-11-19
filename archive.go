package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"net/http"
	"strconv"
)

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

func (app app) generateStatsPageHTML(ctx context.Context, storyID int) ([]byte, error) {
	params := StatsPageParams{
		StoryID: storyID,
	}
	rb := newResponseBuffer()
	req, err := http.NewRequestWithContext(ctx, "GET", "/stats?id="+strconv.Itoa(storyID), nil)
	if err != nil {
		return nil, errors.Wrap(err, "NewRequestWithContext for /stats page")
	}

	// userID is not needed here
	userID := sql.NullInt64{}

	err = app.statsPage(rb, req, params, userID)
	if err != nil {
		return nil, errors.Wrap(err, "app.statsPage")
	}
	if rb.status != http.StatusOK {
		return nil, fmt.Errorf("non-OK HTTP status: %d", rb.status)
	}
	return rb.body, nil
}

func (app app) archiveOldStatsPages(ctx context.Context) ([]int, error) {
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

		filename := fmt.Sprintf("%d.html", storyID)

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

		content, err := app.generateStatsPageHTML(ctx, storyID)
		if err != nil {
			app.logger.Error(fmt.Sprintf("generating stats page for storyId %d", storyID), err)
			continue // Continue with the next storyID
		}

		err = sc.UploadFile(ctx, filename, content)
		if err != nil {
			app.logger.Error(fmt.Sprintf("uploading file %s", filename), err)
			continue // Continue with the next storyID
		}

		app.logger.Debug("Archived stats page for storyID", "storyID", storyID)
		archivedStoryIDs = append(archivedStoryIDs, storyID)
	}

	return archivedStoryIDs, nil
}

func (app app) runArchivingTasks(ctx context.Context) error {
	archivedStoryIDs, err := app.archiveOldStatsPages(ctx)
	if err != nil {
		return errors.Wrap(err, "archiveOldStatsPages")
	}

	nStories := len(archivedStoryIDs)
	nRows, err := app.ndb.deleteOldData(archivedStoryIDs)
	app.logger.Info(fmt.Sprintf("Deleted %d rows from DB for %d stories", nRows, nStories))
	if err != nil {
		return errors.Wrap(err, "deleteOldData")
	}

	return nil
}
