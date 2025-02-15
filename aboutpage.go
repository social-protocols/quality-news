package main

import (
	"net/http"

	"github.com/pkg/errors"
)

type AboutPageData struct {
	PageTemplateData
}

func (d AboutPageData) IsAboutPage() bool {
	return true
}

func (app app) aboutHandler() func(http.ResponseWriter, *http.Request, struct{}) error {
	return func(w http.ResponseWriter, r *http.Request, p struct{}) error {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		err := templates.ExecuteTemplate(w, "about.html.tmpl", AboutPageData{PageTemplateData{UserID: app.getUserID(r)}})

		return errors.Wrap(err, "executing algorithms page template")
	}
}
