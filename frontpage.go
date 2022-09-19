package main

import (
	"net/http"
)

func frontpageHandler() func(w http.ResponseWriter, r *http.Request) {

	// tmpl := template.Must(template.ParseFiles("templates/index.html.tmpl"))

	// stories = ...get stories from rankDatasetDatabaseFilename

	return func(w http.ResponseWriter, r *http.Request) {
		// tmpl.Execute(w, FrontPageData{sampleStories})
	}
}

// frontPageSQL = `
// 	with attentionWithAge as as (
// 		select *, datetime('now','utc')-submissionTime as age
// 		from attention
// 		order by id desc
// 		limit 3000
// 	)
// 	select
// 		id
// 		, upvotes
// 		    / ( cumulativeAttention * (age * age) )
// 		  as score
// 	from attentionWithAge join stories using(id)
// 	order by score desc;
// `
