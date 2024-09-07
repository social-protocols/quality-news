package main

import (
	"net/http"
	"time"
	"fmt"

	"github.com/pkg/errors"
)

const alertAfterMinutes = 5


func (app app) healthHandler() func(http.ResponseWriter, *http.Request, loginParams) error {
	return func(w http.ResponseWriter, r *http.Request, p loginParams) error {

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		lastSampleTime, err := app.ndb.selectLastCrawlTime()
		if err != nil {
			// Return a 500 error
			return errors.Wrap(err, "getting last crawl time")
		}

		if time.Now().Unix()-int64(lastSampleTime) > alertAfterMinutes*60 {
			return fmt.Errorf("last successful crawl of %d is more than %d minutes ago", lastSampleTime, alertAfterMinutes)
		}

		// write "ok"
		_, err = w.Write([]byte("ok"))
		if err != nil {
			return errors.Wrap(err, "writing response")
		}

		return nil
	}
}

