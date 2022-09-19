package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
)

type Story struct {
	ID      int
	By      string
	Title   string
	Url     string
	Age     int
	Upvotes int
	Quality float64
	//	score   float
}

const frontPageSQL = `
	with attentionWithAge as as (
		select *, datetime('now','utc')-submissionTime as age
		from attention
		order by id desc
		limit 3000
	)
	select
		id, by, title, url, age, upvotes
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
		rows, err := statement.Query(statement)
		if err != nil {
			log.Fatal(err)
		}
		defer rows.Close()

		stories := make([]Story, 0, 90)

		for rows.Next() {
			var story Story

			err = rows.Scan(&story.ID, &story.By, &story.Title, &story.Url, &story.Age, &story.Upvotes, &story.Quality)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Got Story", story)
			stories = append(stories, story)

		}
		err = rows.Err()
		if err != nil {
			log.Fatal(err)
		}

		// sampleStory := hn.Item{
		// 	ID: 8863,
		// 	By: "dhouston",
		// 	//              Parent:      8862,
		// 	Title:     "My YC app: Dropbox - Throw away your USB drive",
		// 	URL:       "http://www.getdropbox.com/u/2/screencast.html",
		// 	Timestamp: 1175714200,
		// }
		// sampleStories := []hn.Item{
		// 	sampleStory,
		// }

		tmpl.Execute(w, FrontPageData{stories})
	}
}
