package main

import (
	"fmt"
	"strconv"
	"strings"
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
	Author         string   `selector:"a.hnuser"`
	Score          string   `selector:"span.score"`
	SubmissionTime string   `selector:"span.age" attr:"title"`
	AgeApprox      string   `selector:"span.age"`
	Links          []string `selector:"a"`
}

type ScrapedStory struct {
	Story
	Rank      int
	AgeApprox int64
	Flagged   bool
	Dupe      bool
	Job       bool
}

func (rs rawStory) Clean() (ScrapedStory, error) {
	story := ScrapedStory{
		Story: Story{
			Title: rs.Title,
			By:    rs.Author,
			URL:   rs.URL,
		},
	}

	// parse id
	{
		id, err := strconv.Atoi(rs.ID)
		if err != nil {
			return story, errors.Wrapf(err, "parse story id %s", rs.ID)
		}
		story.ID = id
	}

	// fix url
	if strings.HasPrefix(story.Story.URL, "item?id=") {
		story.Story.URL = "https://news.ycombinator.com/" + story.Story.URL
	}

	// parse score. This field will look like "4 points"
	{
		if fs := strings.Fields(rs.Score); len(fs) > 0 {
			scoreStr := strings.Fields(rs.Score)[0]

			score, err := strconv.Atoi(scoreStr)
			story.Score = score
			if err != nil {
				return story, errors.Wrapf(err, "parse story score %s", rs.Score)
			}
		} else {
			// if there is no upvotes field, then this is an HN job.
			// we want to include these in the database because they get ranked
			story.Job = true
		}
	}

	// parse submission time
	{
		submissionTime, err := time.Parse("2006-01-02T15:04:05", rs.SubmissionTime)
		if err != nil {
			return story, errors.Wrapf(err, "parse submission time %s", rs.SubmissionTime)
		}
		story.SubmissionTime = submissionTime.Unix()
	}

	// parse approximate age
	{
		// this will be something like "1 minute ago" or "3 hours ago"
		if fs := strings.Fields(rs.AgeApprox); len(fs) > 1 {
			n, err := strconv.Atoi(fs[0])
			if err != nil {
				return story, errors.Wrapf(err, "parse relative age %s", rs.AgeApprox)
			}

			var units int64
			if strings.HasPrefix(fs[1], "minute") { // "minute" or "minutes"
				units = 60
			} else if strings.HasPrefix(fs[1], "hour") {
				units = 3600
			} else if strings.HasPrefix(fs[1], "day") {
				units = 3600 * 24
			} else if strings.HasPrefix(fs[1], "month") {
				units = 3600 * 24 * 30
			} else if strings.HasPrefix(fs[1], "year") {
				units = 3600 * 24 * 364
			}

			story.AgeApprox = int64(n) * units
		} else {
			return story, fmt.Errorf("Parse age %s", rs.AgeApprox)
		}

		// parse rank. we know the rank because of the order it appears in.
		// we just use this to do an integrity check later.
		{
			tRank := strings.Trim(rs.Rank, ".")
			var err error
			story.Rank, err = strconv.Atoi(tRank)
			if err != nil || story.Rank == 0 {
				return story, errors.Wrapf(err, "parse rank %s", rs.Rank)
			}
		}

		// parse the number of comments
		{
			// if there are comments, this will be the last <a> tag. Unfortunately, it doesn't have an id or class.
			commentString := rs.Links[len(rs.Links)-1]

			// this string will be a single word like "comment" or "hide" if there are no comments.
			// otherwise it will be something like "12 comments"
			if fs := strings.Fields(commentString); len(fs) > 1 {
				c, err := strconv.Atoi(fs[0])
				if err != nil {
					return story, errors.Wrapf(err, "parse comments %s", commentString)
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
				story.Dupe = true
			}
		}

		return story, nil
	}
}

func (app app) newScraper(resultCh chan ScrapedStory, errCh chan error, moreLinkCh chan string) *colly.Collector {
	c := colly.NewCollector()
	c.SetClient(app.httpClient)

	var rs rawStory

	c.OnHTML("a.morelink", func(e *colly.HTMLElement) {
		moreLinkCh <- e.Attr("href")
	})

	c.OnHTML("tr table", func(e *colly.HTMLElement) {
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
					st, err := rs.Clean()
					rank := st.Rank

					// Do an integrity check. If the row shown for the story equals the row
					// count we are keeping, we area all good.
					if err == nil && ((rank-1)%30)+1 != n {
						err = fmt.Errorf("Ranks out of order. Expected %d but parsed %d", n, (rank-1)%30+1)
					}

					if err != nil {
						app.logger.Debugf("Failed to parse story %d. Raw story %#v", n, rs)
						errCh <- err
					} else {
						resultCh <- st
					}
				}
			}
		})
	})

	c.OnError(func(r *colly.Response, err error) {
		err = errors.Wrapf(err, "Failed to parse page %s", r.Request.URL)
		errCh <- err
	})

	return c
}

// func testScrape() {
// 	localFile := "hacker-news-show-p3-deadlinks.html"

// 	app := initApp()
// 	app.logger.Debug("Doing test scrape")

// 	resultCh := make(chan ScrapedStory)
// 	errCh := make(chan error)
// 	go func() {
// 		t := &http.Transport{}
// 		//		t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
// 		t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))

// 		c := app.newScraper(resultCh, errCh)
// 		c.WithTransport(t)

// 		dir, _ := os.Getwd()
// 		fmt.Println("Visiting", localFile)
// 		_ = c.Visit("file://" + dir + "/" + localFile)
// 	}()

// 	go func() {
// 		app.logger.Debug("Range over errch")
// 		for e := range errCh {
// 			fmt.Println("Go terror", e)
// 		}
// 	}()

// 	app.logger.Debug("Range over resultch")
// 	for result := range resultCh {
// 		fmt.Printf("Got result\n", result)
// 	}
// }

func (app app) scrapeHN(pageType string, resultCh chan ScrapedStory, errCh chan error) {
	baseUrl := "https://news.ycombinator.com/"
	url := baseUrl
	if pageType == "new" {
		url = url + "newest"
	} else if pageType != "top" {
		url = url + pageType
	}
	for p := 1; p <= 3; p++ {
		moreLinkCh := make(chan string, 1)
		c := app.newScraper(resultCh, errCh, moreLinkCh)
		err := c.Visit(url)
		// Sleep a bit to avoid rate limiting
		time.Sleep(520 * time.Millisecond)
		if err != nil {
			errCh <- err
		}
		select {
		case relativeURL := <-moreLinkCh:
			url = baseUrl + relativeURL
		default:
			// there won't always be a next link, in particular the show page could have less than 3 pages worth of stories
		}

	}
	close(resultCh)
	close(errCh)
}
