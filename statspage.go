package main

import (
	"database/sql"
	"html/template"
	"io"
	"net/http"

	"github.com/pkg/errors"

	"github.com/johnwarden/httperror/v2"
)

type StatsPageParams struct {
	StoryID int
}

type StatsPageData struct {
	StatsPageParams
	EstimatedUpvoteRate int
	Story               Story
}

var statsPageTemplate = template.Must(template.ParseFS(resources, "templates/*"))

var ErrStoryIDNotFound = httperror.New(404, "Story ID not found")

func statsPage(ndb newsDatabase, w io.Writer, r *http.Request, params StatsPageParams) error {
	s, err := ndb.selectStoryDetails(params.StoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrStoryIDNotFound
		}
		return err
	}

	d := StatsPageData{
		StatsPageParams:     params, // pass through any and all URL parameters to the template
		EstimatedUpvoteRate: 1.0,
		Story:               s,
	}

	err = statsPageTemplate.ExecuteTemplate(w, "stats.html.tmpl", d)

	return errors.Wrap(err, "executing stats page template")
}
