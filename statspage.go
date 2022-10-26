package main

import (
	"net/http"
	"io"


	"html/template"
	"github.com/pkg/errors"
)

type StatsPageParams struct {
	StoryID int
}

type StatsPageData struct {
	StatsPageParams
	EstimatedUpvoteRate int
	Title string
}

var statsPageTemplate = template.Must(template.ParseFS(resources, "templates/*"))

func statsPage(w io.Writer, r *http.Request, params StatsPageParams) error {


	d := StatsPageData {
		StatsPageParams: params, // pass through any and all URL parameters to the template
		EstimatedUpvoteRate: 1.0,
		Title: "TODO: lookup story title",
	}

	err := statsPageTemplate.ExecuteTemplate(w, "stats.html.tmpl", d)

	return errors.Wrap(err, "executing stats page template")
}

