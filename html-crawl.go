package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gocolly/colly/v2"
)

var url = "http://webcache.googleusercontent.com/search?q=cache%3Ahttps%3A%2F%2Fnews.ycombinator.com%2F&oq=cache%3Ahttps%3A%2F%2Fnews.ycombinator.com%2F&aqs=chrome..69i57j69i58.13944j0j9&sourceid=chrome&ie=UTF-8"

// var url = "https://news.ycombinator.com/"

type RawStory struct {
	ID string
	Line1
	Line2
}

type Line1 struct {
	Title string `selector:"span.titleline a"`
	URL   string `selector:"span.titleline a" attr:"href"`
}

type Line2 struct {
	Author string `selector:"a.hnuser"`
	//	Author2 string `selector:"span.subline"`
	Score string `selector:"span.score"`
	//	Subtext string `selector:"td.subtext"`
	Age string `selector:"span.age" attr:"title"`
}

func main() {
	c := colly.NewCollector()

	c.OnHTML(".itemlist", func(e *colly.HTMLElement) {
		// i := 0;

		var stories [90]RawStory

		e.ForEach("tr", func(i int, e *colly.HTMLElement) {
			// There are three trs for each story
			n := i / 3

			if i%3 == 0 {
				stories[n].ID = e.Attr("id")
				err := e.Unmarshal(&stories[n].Line1)
				if err != nil {
					fmt.Println("Error", err)
					log.Fatal(err)
				}
			}
			if i%3 == 1 {
				err := e.Unmarshal(&stories[n].Line2)
				if err != nil {
					fmt.Println("Error", err)
					log.Fatal(err)
				}
				fmt.Printf("Got story at %d, %d, %#v\n", i, n, stories[n])
			}

			if n > 1 {
				os.Exit(2)
			}
		})
	})

	c.OnError(func(r *colly.Response, err error) {
		fmt.Println("Request URL:", r.Request.URL, "failed with response:", r, "\nError:", err)
	})

	c.Visit(url)
}
