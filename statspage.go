package main

import (
	"database/sql"
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
	RanksPlotData   [][]any
	UpvotesPlotData [][]any
	PenaltyPlotData [][]any
	MaxSampleTime   int
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
	if err != nil {
		return errors.Wrap(err, "maxSampleTime")
	}
	d.MaxSampleTime = maxSampleTime

	ranks, err := rankDatapoints(ndb, params.StoryID)
	if err != nil {
		return errors.Wrap(err, "rankDatapoints")
	}
	d.RanksPlotData = ranks

	upvotes, err := upvotesDatapoints(ndb, params.StoryID, modelParams)
	if err != nil {
		return errors.Wrap(err, "upvotesDatapoints")
	}
	d.UpvotesPlotData = upvotes

	err = templates.ExecuteTemplate(w, "stats.html.tmpl", d)

	return errors.Wrap(err, "executing stats page template")
}
