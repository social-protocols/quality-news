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
type StatsPageData struct {
	StatsPageParams
	DefaultPageHeaderData
	EstimatedUpvoteRate int
	Story
	RanksPlotDataJSON   template.JS
	UpvotesPlotDataJSON template.JS
	PenaltyPlotDataJSON template.JS
	MaxSampleTime       int
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

	ndb := app.ndb
	s, err := ndb.selectStoryDetails(params.StoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrStoryIDNotFound
		}
		return err
	}

	modelParams := params.OptionalModelParams.WithDefaults()
	s.UpvoteRate = modelParams.upvoteRate(s.CumulativeUpvotes, s.CumulativeExpectedUpvotes)

	s.IsStatsPage = true

	d := StatsPageData{
		StatsPageParams:       params, // pass through any and all URL parameters to the template
		EstimatedUpvoteRate:   1.0,
		Story:                 s,
		DefaultPageHeaderData: DefaultPageHeaderData{UserID: userID},
	}

	maxSampleTime, err := maxSampleTime(ndb, params.StoryID)
	d.MaxSampleTime = maxSampleTime

	if s.Archived {
		app.logger.Debug("Loading story data from archive", "storyID", params.StoryID)
		err = app.loadStatsDataFromArchive(params.StoryID, &d)
		if err != nil {
			return errors.Wrap(err, "loading stats data from archive")
		}
	} else {

		ranks, err := rankDatapoints(ndb, params.StoryID)
		if err != nil {
			return errors.Wrap(err, "rankDatapoints")
		}
		ranksJson, err := json.Marshal(ranks)
		if err != nil {
			return errors.Wrap(err, "json.Marshal RanksPlotData")
		}
		d.RanksPlotDataJSON = template.JS(string(ranksJson))

		upvotes, err := upvotesDatapoints(ndb, params.StoryID, modelParams)
		if err != nil {
			return errors.Wrap(err, "upvotesDatapoints")
		}
		upvotesJson, err := json.Marshal(upvotes)
		if err != nil {
			return errors.Wrap(err, "json.Marshal UpvotesPlotData")
		}
		d.UpvotesPlotDataJSON = template.JS(string(upvotesJson))

	}

	err = templates.ExecuteTemplate(w, "stats.html.tmpl", d)
	return errors.Wrap(err, "executing stats page template")
}

func (app app) loadStatsDataFromArchive(storyID int, d *StatsPageData) error {
	// Create storage client
	sc, err := NewStorageClient()
	if err != nil {
		return errors.Wrap(err, "creating storage client")
	}

	// Download JSON file
	filename := fmt.Sprintf("%d.json", storyID)
	jsonData, err := sc.DownloadFile(context.Background(), filename)
	if err != nil {
		return errors.Wrap(err, "downloading JSON file from archive")
	}

	// Unmarshal JSON data into StatsData struct
	var statsData StatsData
	err = json.Unmarshal(jsonData, &statsData)
	if err != nil {
		return errors.Wrap(err, "json.Unmarshal statsData")
	}

	// Set data into StatsPageData
	d.MaxSampleTime = statsData.MaxSampleTime

	// Marshal data to JSON strings and assign to template fields
	ranksPlotDataJSON, err := json.Marshal(statsData.RanksPlotData)
	if err != nil {
		return errors.Wrap(err, "json.Marshal RanksPlotData")
	}
	upvotesPlotDataJSON, err := json.Marshal(statsData.UpvotesPlotData)
	if err != nil {
		return errors.Wrap(err, "json.Marshal UpvotesPlotData")
	}

	d.RanksPlotDataJSON = template.JS(ranksPlotDataJSON)
	d.UpvotesPlotDataJSON = template.JS(upvotesPlotDataJSON)
	// Handle PenaltyPlotData similarly

	return nil
}
