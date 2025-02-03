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
	DefaultPageHeaderData
	EstimatedUpvoteRate int
	Story
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
	s.IsStatsPage = true

	d := StatsPageData{
		StatsPageParams:       params,
		EstimatedUpvoteRate:   1.0,
		Story:                 s,
		DefaultPageHeaderData: DefaultPageHeaderData{UserID: userID},
		StatsData:             stats,
	}

	err = templates.ExecuteTemplate(w, "stats.html.tmpl", d)
	return errors.Wrap(err, "executing stats page template")
}

func (app app) loadStoryAndStats(ctx context.Context, storyID int, modelParams OptionalModelParams) (Story, StatsData, error) {
	ndb := app.ndb

	// Try to get story from DB first
	s, err := ndb.selectStoryDetails(storyID)

	// for debugging
	// err = sql.ErrNoRows

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			app.logger.Debug("Story not found in DB, trying archives", "storyID", storyID)
			// Story not in DB, try to load from archive
			sc, err := NewStorageClient()
			if err != nil {
				return Story{}, StatsData{}, errors.Wrap(err, "create storage client")
			}

			// Try v2 archive first
			filename := fmt.Sprintf("%d.v2.json", storyID)
			jsonData, err := sc.DownloadFile(ctx, filename)
			isV2 := err == nil
			if err != nil {
				app.logger.Debug("V2 archive not found, trying legacy format", "storyID", storyID)
				// Try legacy archive
				filename = fmt.Sprintf("%d.json", storyID)
				jsonData, err = sc.DownloadFile(ctx, filename)
				if err != nil {
					app.logger.Debug("Story not found in any archive", "storyID", storyID)
					return Story{}, StatsData{}, ErrStoryIDNotFound
				}
				app.logger.Debug("Found story in legacy archive", "storyID", storyID)
			} else {
				app.logger.Debug("Found story in v2 archive", "storyID", storyID)
			}

			var archiveData ArchiveData
			err = json.Unmarshal(jsonData, &archiveData)
			if err != nil {
				return Story{}, StatsData{}, errors.Wrap(err, "unmarshal archive data")
			}

			// For v2 archives, construct Story from archive data
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
		return Story{}, StatsData{}, err
	}

	app.logger.Debug("Story found in DB", "storyID", storyID, "archived", s.Archived)

	// Story found in DB
	if s.Archived {
		// Story is archived in legacy format (v2 format deletes from DB)
		sc, err := NewStorageClient()
		if err != nil {
			return Story{}, StatsData{}, errors.Wrap(err, "create storage client")
		}

		app.logger.Debug("Loading legacy archive data", "storyID", storyID)
		// Only try legacy format since story is still in DB
		filename := fmt.Sprintf("%d.json", storyID)
		jsonData, err := sc.DownloadFile(ctx, filename)
		if err != nil {
			return Story{}, StatsData{}, fmt.Errorf("Missing archive file for story id %d", storyID)
		}

		var archiveData ArchiveData
		err = json.Unmarshal(jsonData, &archiveData)
		if err != nil {
			return Story{}, StatsData{}, errors.Wrap(err, "unmarshal archive data")
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

	app.logger.Debug("Loading stats from DB", "storyID", storyID)
	// Story is not archived, get stats from DB
	maxSampleTime, err := maxSampleTime(ndb, storyID)
	if err != nil {
		return Story{}, StatsData{}, errors.Wrap(err, "maxSampleTime")
	}

	ranks, err := rankDatapoints(ndb, storyID)
	if err != nil {
		return Story{}, StatsData{}, errors.Wrap(err, "rankDatapoints")
	}

	ranksJson, err := json.Marshal(ranks)
	if err != nil {
		return Story{}, StatsData{}, errors.Wrap(err, "marshal ranks plot data")
	}

	upvotes, err := upvotesDatapoints(ndb, storyID, modelParams.WithDefaults())
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
