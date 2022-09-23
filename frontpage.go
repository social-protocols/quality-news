package main

import (
	"embed"
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
	ID       int
	By       string
	Title    string
	URL      string
	Age      string
	Upvotes  int
	Comments int
	Quality  string
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
		id, by, title, url, submissionTime, totalUpvotes, totalComments
		, (upvotes + 2.2956)/(cumulativeAttention+2.2956) as quality 
	from attentionWithAge join stories using(id)
	order by 
		quality desc
	limit 90;

*/
`

/* The Bayesian averaging constant/formula from bayesian-average-quality.R
   (in the hacker-news-data repo).

   stories$bayesianAverageLogQuality = (log(stories$qualityRatio)*stories$upvotes) / (stories$upvotes + k)

   The constant value k from running this model on a sample of 100 stories is 2.2956

   To make this more readable:

   		q = quality
   		a = attention
   		v = upvotes
   		k = constant (strength of prior)

   		log(q) = log(v/a)*a / ( a + k )


   Now we want the quality, not log quality. With a little math, we get

   		q = (v/a)^(v/(v+k))
*/

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

func frontpageHandler(ndb newsDatabase) func(w http.ResponseWriter, r *http.Request) {

	statement, err := ndb.db.Prepare(frontPageSQL) // Prepare statement.
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
