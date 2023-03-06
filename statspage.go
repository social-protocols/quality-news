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

func statsPage(ndb newsDatabase, w io.Writer, r *http.Request, params StatsPageParams) error {
	s, err := ndb.selectStoryDetails(params.StoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrStoryIDNotFound
		}
		return err
	}

	s.IsStatsPage = true

	d := StatsPageData{
		StatsPageParams:     params, // pass through any and all URL parameters to the template
		EstimatedUpvoteRate: 1.0,
		Story:               s,
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

	upvotes, err := upvotesDatapoints(ndb, params.StoryID)
	if err != nil {
		return errors.Wrap(err, "upvotesDatapoints")
	}
	d.UpvotesPlotData = upvotes

	err = templates.ExecuteTemplate(w, "stats.html.tmpl", d)

	return errors.Wrap(err, "executing stats page template")
}
