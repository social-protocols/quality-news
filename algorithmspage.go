package main

import (
	"net/http"

	"github.com/pkg/errors"
)

type AlgorithmsPageData struct {
	PageTemplateData
}

func (d AlgorithmsPageData) IsAlgorithmsPage() bool {
	return true
}

func (app app) algorithmsHandler() func(http.ResponseWriter, *http.Request, struct{}) error {
	return func(w http.ResponseWriter, r *http.Request, p struct{}) error {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		err := templates.ExecuteTemplate(w, "about.html.tmpl", AlgorithmsPageData{PageTemplateData{UserID: app.getUserID(r)}})

		return errors.Wrap(err, "executing Algorithms page template")
	}
}
