package main

import (
	"database/sql"
	"io"
	"net/http"

	"github.com/pkg/errors"

	"github.com/johnwarden/httperror"
)

type StatsPageParams struct {
	StoryID int `schema:"id,required"`
}

type StatsPageData struct {
	StatsPageParams
	EstimatedUpvoteRate int
	Story               Story
	RanksPlotData       [][]any
	UpvotesPlotData     [][]any
	PenaltyPlotData     [][]any
}

func (d StatsPageData) IsQualityPage() bool {
	return false
}

func (d StatsPageData) IsHNTopPage() bool {
	return false
}

func (d StatsPageData) IsNewPage() bool {
	return false
}

func (d StatsPageData) IsBestPage() bool {
	return false
}

func (d StatsPageData) IsAskPage() bool {
	return false
}

func (d StatsPageData) IsShowPage() bool {
	return false
}

func (d StatsPageData) IsRawPage() bool {
	return false
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
