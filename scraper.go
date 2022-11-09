package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/pkg/errors"
)

type rawStory struct {
	ID string
	row1
	row2
}

type row1 struct {
	Title     string `selector:"span.titleline a"`
	FullTitle string `selector:"span.titleline"`
	URL       string `selector:"span.titleline a" attr:"href"`
	Rank      string `selector:"span.rank"`
}

type row2 struct {
	Author string   `selector:"a.hnuser"`
	Score  string   `selector:"span.score"`
	Age    string   `selector:"span.age" attr:"title"`
	Links  []string `selector:"a"`
}

type ScrapedStory struct {
	Rank int
	Story
}

func (rs rawStory) Clean() (Story, int, error) {
	story := Story{
		Title: rs.Title,
		URL:   rs.URL,
		By:    rs.Author,
	}

	// parse id
	{
		id, err := strconv.Atoi(rs.ID)
		if err != nil {
			return story, 0, errors.Wrapf(err, "parse story id %s", rs.ID)
		}
		story.ID = id
	}

	// parse secore
	// field will look like "4 points"
	{
		if fs := strings.Fields(rs.Score); len(fs) > 0 {
			scoreStr := strings.Fields(rs.Score)[0]

			score, err := strconv.Atoi(scoreStr)
			story.Upvotes = score - 1
			if err != nil {
				return story, 0, errors.Wrapf(err, "parse story score %s", rs.Score)
			}
		}
		// if there is no upvotes field, it is a job
		// we want to treat these as stories because they get ranked
	}

	// parse submission time
	{
		submissionTime, err := time.Parse("2006-01-02T15:04:05", rs.Age)
		if err != nil {
			return story, 0, errors.Wrapf(err, "parse submission time %s", rs.Age)
		}
		story.SubmissionTime = submissionTime.Unix()
	}

	// parse rank. we know the rank because of the order it appears in.
	// we just use this to do an integrity check later.
	rank := 0
	{
		tRank := strings.Trim(rs.Rank, ".")
		var err error
		rank, err = strconv.Atoi(tRank)
		if err != nil || rank == 0 {
			return story, 0, errors.Wrapf(err, "parse rank %s", rs.Rank)
		}
	}

	// parse the number of comments
	{
		// if there are comments, this will be the last <a> tag
		commentString := rs.Links[len(rs.Links)-1]

		// this string will be a single word like "comment" or "hide" if there are no comments.
		if fs := strings.Fields(commentString); len(fs) > 1 {
			c, err := strconv.Atoi(fs[0])
			if err != nil {
				return story, 0, errors.Wrapf(err, "parse comments", commentString)
			}
			story.Comments = c
		}
	}

	// parse [flagged] and [dupe] tags
	{
		if strings.Contains(rs.FullTitle, "[flagged]") {
			story.Flagged = true
		}
		if strings.Contains(rs.FullTitle, "[dupe]") {
			story.Duplicate = true
		}
	}

	return story, rank, nil
}

func (app app) newScraper(logger leveledLogger, resultCh chan ScrapedStory, errCh chan error) *colly.Collector {
	c := colly.NewCollector()
	c.SetClient(app.httpClient)

	var rs rawStory

	c.OnHTML(".itemlist", func(e *colly.HTMLElement) {
		n := 0
		lastStoryRownum := 0
		e.ForEach("tr", func(i int, e *colly.HTMLElement) {
			class := e.Attr("class")

			// stories will always start with a tr of class athing
			if class == "athing" && n < 30 {
				n = n + 1
				lastStoryRownum = i
				if n > 30 {
					return
				}

				rs = rawStory{
					ID: e.Attr("id"),
				}
				err := e.Unmarshal(&rs.row1)
				if err != nil {
					errCh <- err
				}
			} else if class == "" && i == lastStoryRownum+1 && n > 0 && n <= 30 {

				// the first tr after the "athing" contains the second row of
				// details for the story. Note also we must skip any trs
				// before the first athing because sometimes they contain
				// general page content.

				err := e.Unmarshal(&rs.row2)

				if err != nil {
					errCh <- err
				} else {
					s, rank, err := rs.Clean()

					// Do an integrity check. If the row shown for the story equals the row
					// count we are keeping, we area all good.
					if err == nil && ((rank-1)%30)+1 != n {
						err = fmt.Errorf("Ranks out of order. Expected %d but parsed %d", n, (rank-1)%30+1)
					}

					if err != nil {
						logger.Debugf("Failed to parse story %d. Raw story %#v", n, rs)
						errCh <- err
					} else {
						resultCh <- ScrapedStory{rank, s}
					}
				}
			}
		})
	})

	c.OnError(func(r *colly.Response, err error) {
		err = errors.Wrapf(err, "Failed to parse URL %s", r.Request.URL)
		errCh <- err
	})

	return c
}

func testScrape() {
	localFile := "hacker-news-show-p3-deadlinks.html"

	app := initApp()
	app.logger.Debug("Doing test scrape")

	resultCh := make(chan ScrapedStory)
	errCh := make(chan error)
	go func() {
		t := &http.Transport{}
		//		t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
		t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))

		c := app.newScraper(app.logger, resultCh, errCh)
		c.WithTransport(t)

		dir, _ := os.Getwd()
		fmt.Println("Visiting", localFile)
		c.Visit("file://" + dir + "/" + localFile)
	}()

	go func() {
		app.logger.Debug("Range over errch")
		for e := range errCh {
			fmt.Println("Go terror", e)
		}
	}()

	app.logger.Debug("Range over resultch")
	for result := range resultCh {
		fmt.Printf("Got result %#v\n", result)
	}
}

func (app app) scrapeHN(pageType string, resultCh chan ScrapedStory, errCh chan error) {
	logger := app.logger
	url := "https://news.ycombinator.com/"
	if pageType == "new" {
		url = url + "newest"
	} else if pageType != "top" {
		url = url + pageType
	}
	var wg sync.WaitGroup
	for p := 1; p <= 3; p++ {
		wg.Add(1)
		go func(page int) {
			defer wg.Done()
			c := app.newScraper(logger, resultCh, errCh)
			u := url
			if page > 1 {
				u = url + "?p=" + strconv.Itoa(page)
			}
			c.Visit(u)
		}(p)
	}
	wg.Wait()
	close(resultCh)
	close(errCh)
}
