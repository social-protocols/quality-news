package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"time"

	"github.com/pkg/errors"

	"github.com/johnwarden/httperror"
)

type StatsPageParams struct {
	StoryID int `schema:"id,required"`
	OptionalModelParams
}

type StatsData struct {
	RanksPlotDataJSON   template.JS
	UpvotesPlotDataJSON template.JS
	MaxSampleTime       int
}

type StatsPageData struct {
	StatsPageParams
	EstimatedUpvoteRate int
	StoryTemplateData
	StatsData
}

func (s StatsPageData) MaxSampleTimeISOString() string {
	return time.Unix(int64(s.MaxSampleTime), 0).UTC().Format("2006-01-02T15:04")
}

func (s StatsPageData) OriginalSubmissionTimeISOString() string {
	return time.Unix(s.OriginalSubmissionTime, 0).UTC().Format("2006-01-02T15:04")
}

func (s StatsPageData) MaxAgeHours() int {
	return (s.MaxSampleTime - int(s.OriginalSubmissionTime)) / 3600
}

var ErrStoryIDNotFound = httperror.New(404, "Story ID not found")

func (app app) statsPage(w io.Writer, r *http.Request, params StatsPageParams, userID sql.NullInt64) error {
	s, stats, err := app.loadStoryAndStats(r.Context(), params.StoryID, params.OptionalModelParams)
	if err != nil {
		return err
	}

	modelParams := params.OptionalModelParams.WithDefaults()
	s.UpvoteRate = modelParams.upvoteRate(s.CumulativeUpvotes, s.CumulativeExpectedUpvotes)

	pageTemplate := PageTemplateData{
		UserID: userID,
	}

	storyTemplate := StoryTemplateData{
		Story:            s,
		PageTemplateData: pageTemplate,
	}

	d := StatsPageData{
		StatsPageParams:     params,
		EstimatedUpvoteRate: 1.0,
		StoryTemplateData:   storyTemplate,
		StatsData:           stats,
	}

	err = templates.ExecuteTemplate(w, "stats.html.tmpl", d)
	return errors.Wrap(err, "executing stats page template")
}

func (app app) loadStoryAndStats(ctx context.Context, storyID int, modelParams OptionalModelParams) (Story, StatsData, error) {
	ndb := app.ndb

	// Try to get story from DB first
	s, err := ndb.selectStoryDetails(ctx, storyID)
	dbRecordExists := (err == nil)
	isArchived := (dbRecordExists && s.Archived)

	// If story doesn't exist in DB or is archived, try to load from archive
	if !dbRecordExists || isArchived {
		app.logger.Info("Loading story from archive", "storyID", storyID, "dbRecordExists", dbRecordExists, "isArchived", isArchived)

		sc, err := NewStorageClient()
		if err != nil {
			return Story{}, StatsData{}, errors.Wrap(err, "create storage client")
		}

		// Try v2 archive first
		filename := fmt.Sprintf("%d.v2.json", storyID)
		jsonData, err := sc.DownloadFile(ctx, filename)
		isV2 := err == nil
		if err != nil {
			// Try legacy archive
			filename = fmt.Sprintf("%d.json", storyID)
			jsonData, err = sc.DownloadFile(ctx, filename)
			if err != nil {
				if !dbRecordExists {
					return Story{}, StatsData{}, ErrStoryIDNotFound
				}
				return Story{}, StatsData{}, errors.Wrapf(err, "failed to load archive file %s for story marked as archived", filename)
			}
		}

		var archiveData ArchiveData
		err = json.Unmarshal(jsonData, &archiveData)
		if err != nil {
			return Story{}, StatsData{}, errors.Wrap(err, "unmarshal archive data")
		}

		if isV2 {
			// Calculate AgeApprox as current time minus submission time
			ageApprox := time.Now().Unix() - archiveData.SubmissionTime

			s = Story{
				ID:                        archiveData.ID,
				By:                        archiveData.By,
				Title:                     archiveData.Title,
				URL:                       archiveData.URL,
				SubmissionTime:            archiveData.SubmissionTime,
				OriginalSubmissionTime:    archiveData.OriginalSubmissionTime,
				AgeApprox:                 ageApprox,
				Score:                     archiveData.Score,
				Comments:                  archiveData.Comments,
				CumulativeUpvotes:         archiveData.CumulativeUpvotes,
				CumulativeExpectedUpvotes: archiveData.CumulativeExpectedUpvotes,
				TopRank:                   archiveData.TopRank,
				QNRank:                    archiveData.QNRank,
				RawRank:                   archiveData.RawRank,
				Flagged:                   archiveData.Flagged,
				Dupe:                      archiveData.Dupe,
				Job:                       archiveData.Job,
				Archived:                  archiveData.Archived,
			}
		} else {
			// For legacy archives, we need story details from DB
			return Story{}, StatsData{}, ErrStoryIDNotFound
		}

		// Convert plot data to JSON
		ranksJson, err := json.Marshal(archiveData.RanksPlotData)
		if err != nil {
			return Story{}, StatsData{}, errors.Wrap(err, "marshal ranks plot data")
		}

		upvotesJson, err := json.Marshal(archiveData.UpvotesPlotData)
		if err != nil {
			return Story{}, StatsData{}, errors.Wrap(err, "marshal upvotes plot data")
		}

		stats := StatsData{
			RanksPlotDataJSON:   template.JS(string(ranksJson)),
			UpvotesPlotDataJSON: template.JS(string(upvotesJson)),
			MaxSampleTime:       archiveData.MaxSampleTime,
		}

		return s, stats, nil
	}

	// Story is not archived, get stats from DB
	maxSampleTime, err := maxSampleTime(ctx, ndb, storyID)
	if err != nil {
		return Story{}, StatsData{}, errors.Wrap(err, "maxSampleTime")
	}

	ranks, err := rankDatapoints(ctx, ndb, storyID)
	if err != nil {
		return Story{}, StatsData{}, errors.Wrap(err, "rankDatapoints")
	}

	ranksJson, err := json.Marshal(ranks)
	if err != nil {
		return Story{}, StatsData{}, errors.Wrap(err, "marshal ranks plot data")
	}

	upvotes, err := upvotesDatapoints(ctx, ndb, storyID, modelParams.WithDefaults())
	if err != nil {
		return Story{}, StatsData{}, errors.Wrap(err, "upvotesDatapoints")
	}

	upvotesJson, err := json.Marshal(upvotes)
	if err != nil {
		return Story{}, StatsData{}, errors.Wrap(err, "marshal upvotes plot data")
	}

	stats := StatsData{
		RanksPlotDataJSON:   template.JS(string(ranksJson)),
		UpvotesPlotDataJSON: template.JS(string(upvotesJson)),
		MaxSampleTime:       maxSampleTime,
	}

	return s, stats, nil
}
