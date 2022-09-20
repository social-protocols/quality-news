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

type frontPageData struct {
	Stories []story
}

type story struct {
	ID      int
	By      string
	Title   string
	URL     string
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

		stories := make([]story, 0, 90)

		for rows.Next() {
			var s story

			var submissionTime int
			err = rows.Scan(&s.ID, &s.By, &s.Title, &s.URL, &submissionTime, &s.Upvotes, &s.Quality)

			ageString := humanize.Time(time.Unix(int64(submissionTime), 0))
			s.Age = ageString

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

		err = tmpl.Execute(w, frontPageData{stories})
		if err != nil {
			fmt.Println(err)
		}
	}
}
