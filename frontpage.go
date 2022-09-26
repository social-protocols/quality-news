package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
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
  order by quality / pow(cast(unixepoch()-submissionTime as real)/3600 + 2, 1.2) desc
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

func frontpageHandler(ndb newsDatabase, ranking string) func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	var sql string
	if ranking == "quality" {
		sql = frontPageSQL
	} else if ranking == "hntop" {
		sql = hnTopPageSQL
	}

	statement, err := ndb.db.Prepare(sql)
	if err != nil {
		log.Fatal(err)
	}

	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		rows, err := statement.Query()
		if err != nil {
			fmt.Println("Failed to get front page")
			log.Fatal(err)
		}
		defer rows.Close()

		stories := make([]story, 0, 90)

		for rows.Next() {
			var s story

			var submissionTime int
			var quality float64
			err = rows.Scan(&s.ID, &s.By, &s.Title, &s.URL, &submissionTime, &s.Upvotes, &s.Comments, &quality)

			ageString := humanize.Time(time.Unix(int64(submissionTime), 0))
			s.Age = ageString

			s.Quality = fmt.Sprintf("%.2f", quality)

			if err != nil {
				fmt.Println("Failed to scan row")
				log.Fatal(err)
			}
			stories = append(stories, s)

		}
		err = rows.Err()
		if err != nil {
			log.Fatal(err)
		}

		err = t.ExecuteTemplate(w, "index.html.tmpl", frontPageData{stories})
		if err != nil {
			fmt.Println(err)
		}
	}
}
