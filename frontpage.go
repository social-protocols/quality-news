package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	humanize "github.com/dustin/go-humanize"
)

// FrontPageData contains the data to populate the front page template.
type FrontPageData struct {
	Stories []Story
}

type Story struct {
	ID      int
	By      string
	Title   string
	Url     string
	Age     string
	Upvotes int
	Quality float64
	//	score   float
}

const frontPageSQL = `
	with attentionWithAge as (
		select *, unixepoch()-submissionTime as age
		from attention
		order by id desc
		limit 3000
	)
	select
		id, by, title, url, submissionTime, upvotes
		, upvotes/cumulativeAttention as quality 
	from attentionWithAge join stories using(id)
	order by 
		upvotes
			/ ( cumulativeAttention * (age * age) )
	    desc
	limit 90;
`

func frontpageHandler(db *sql.DB) func(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles("templates/index.html.tmpl"))

	statement, err := db.Prepare(frontPageSQL) // Prepare statement.
	if err != nil {
		log.Fatal(err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := statement.Query()
		if err != nil {
			fmt.Println("Failed to get front page")
			log.Fatal(err)
		}
		defer rows.Close()

		stories := make([]Story, 0, 90)

		for rows.Next() {
			var story Story

			var submissionTime int
			err = rows.Scan(&story.ID, &story.By, &story.Title, &story.Url, &submissionTime, &story.Upvotes, &story.Quality)

			ageString := humanize.Time(time.Unix(int64(submissionTime), 0))
			story.Age = ageString

			if err != nil {
				fmt.Println("Failed to scan row")
				log.Fatal(err)
			}
			stories = append(stories, story)

		}
		err = rows.Err()
		if err != nil {
			log.Fatal(err)
		}

		err = tmpl.Execute(w, FrontPageData{stories})
		if err != nil {
			fmt.Println(err)
		}
	}
}
