package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"embed"
	"fmt"
	"github.com/pkg/errors"
	"html/template"
	"net/http"
	"strconv"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/julienschmidt/httprouter"
)

type frontPageData struct {
	Stories []story
}

type story struct {
	ID       int
	By       string
	Title    string
	URL      string
	Age      string
	Upvotes  int
	Comments int
	Quality  string
}

const defaultGravity = 0.8

const frontPageSQL = `
  select
    id
    , by
    , title
    , url
    , submissionTime
    , score
    , descendants
    , (upvotes + 2.2956)/(cumulativeAttention+2.2956) as quality 
  from attention
  join stories using(id)
  join dataset using(id)
  where sampleTime = (select max(sampleTime) from dataset)
  order by quality / pow(cast(unixepoch()-submissionTime as real)/3600 + 2, %f) desc
  limit 90;
`

const hnTopPageSQL = `
  select
    id
    , by
    , title
    , url
    , submissionTime
    , score
    , descendants
    , (upvotes + 2.2956)/(cumulativeAttention+2.2956) as quality 
  from attention
  join stories using(id)
  join dataset using(id)
  where sampleTime = (select max(sampleTime) from dataset) and toprank is not null
  order by toprank asc
  limit 90;
`

/* The constant k comes from bayesian-average-quality.R (in the hacker-news-data repo).

   Bayesian Average Quality Formula

   	quality ≈ (upvotes+k)/(cumulativeAttention+k)

   Then add age. We want the age penalty to mimic the original HN formula:

	   pow(upvotes, 0.8) / pow(ageHours + 2, 1.8)

	The age penalty actually serves two purposes: 1) a proxy for attention and 2) to make
	sure stories cycle through the home page.

	But if we find that cumulativeAttention roughly equals ageHours^f, then an
	age penalty is already "built in" to our formula. But our guess is that
	f is something like 0.6, so we need to add an addition penalty of:


		(ageHours+2)^(1.8-f)

	So the ranking formula is:

   	quality ≈ (upvotes+k)/(cumulativeAttention+k)/(ageHours+2)^(1.8-f)

*/

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

var pages map[string][]byte
var statements map[string]*sql.Stmt

func renderFrontPages(ndb newsDatabase, logger leveledLogger) error {
	rankings := []string{"quality", "hntop"}

	for _, ranking := range rankings {
		bytes, err := renderFrontPage(ndb, logger, ranking, -1)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("render %s page", ranking))
		}
		pages[ranking] = bytes
	}

	return nil
}

func renderFrontPage(ndb newsDatabase, logger leveledLogger, ranking string, gravity float64) ([]byte, error) {

	logger.Info("Rendering front page", "ranking", ranking)

	stories, err := getFrontPageStories(ndb, ranking, gravity)
	if err != nil {
		return nil, errors.Wrap(err, "getFrontPageStories")
	}

	var b bytes.Buffer

	zw := gzip.NewWriter(&b)
	defer zw.Close()

	if err = t.ExecuteTemplate(zw, "index.html.tmpl", frontPageData{stories}); err != nil {
		return nil, errors.Wrap(err, "executing front page template")
	}

	if pages == nil {
		pages = make(map[string][]byte)
	}
	zw.Close()

	return b.Bytes(), nil
}

func getFrontPageStories(ndb newsDatabase, ranking string, gravity float64) (stories []story, err error) {

	if statements == nil {
		statements = make(map[string]*sql.Stmt)
	}

	var s *sql.Stmt

	// Prepare statement if it hasn't already been prepared or if we are using
	// custom gravity
	if statements[ranking] == nil || gravity != -1 {
		var sql string
		if ranking == "quality" {
			sql = frontPageSQL
		} else if ranking == "hntop" {
			sql = hnTopPageSQL
		}

		if gravity == -1 {
			gravity = defaultGravity
		}

		s, err = ndb.db.Prepare(fmt.Sprintf(sql, gravity))
		if err != nil {
			return stories, errors.Wrap(err, "preparing front page SQL")
		}

		if gravity == -1 {
			statements[ranking] = s
		}
	} else {
		s = statements[ranking]
	}

	rows, err := s.Query()
	if err != nil {
		return stories, errors.Wrap(err, "executing front page SQL")
	}
	defer rows.Close()

	for rows.Next() {
		var s story

		var submissionTime int
		var quality float64
		err = rows.Scan(&s.ID, &s.By, &s.Title, &s.URL, &submissionTime, &s.Upvotes, &s.Comments, &quality)

		ageString := humanize.Time(time.Unix(int64(submissionTime), 0))
		s.Age = ageString

		s.Quality = fmt.Sprintf("%.2f", quality)

		if err != nil {
			return stories, errors.Wrap(err, "Scanning row")
		}
		stories = append(stories, s)

	}

	err = rows.Err()
	if err != nil {
		return stories, err
	}

	return stories, nil

}

func frontpageHandler(ndb newsDatabase, ranking string, logger leveledLogger) func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Encoding", "gzip")

		gravityStr := r.URL.Query().Get("gravity")

		var b []byte
		var err error
		if gravityStr != "" {
			gravity, err := strconv.ParseFloat(gravityStr, 64)
			if err != nil {
				logger.Err(errors.Wrap(err, "ParseFloat(gravityStr)"))
				w.WriteHeader(400)
				w.Write([]byte("bad request"))
				return
			}
			logger.Info("Generating front page with custom gravity", "gravity", gravity)
			b, err = renderFrontPage(ndb, logger, ranking, gravity)
			if err != nil {
				w.WriteHeader(500)
				logger.Err(errors.Wrap(err, "renderFrontPage"))
				w.Write([]byte("internal server error"))
				return
			}
		} else {
			b = pages[ranking]
		}

		_, err = w.Write(b)

		if err != nil {
			w.WriteHeader(500)
			logger.Err(errors.Wrap(err, "writeFrontPage"))
			w.Write([]byte("internal server error"))
		}

	}
}
