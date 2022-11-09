package main

import (
	"fmt"
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
	Author      string   `selector:"a.hnuser"`
	Score       string   `selector:"span.score"`
	Age         string   `selector:"span.age" attr:"title"`
	RelativeAge string   `selector:"span.age"`
	Links       []string `selector:"a"`
}

type ScrapedStory struct {
	Rank int
	Story
	Resubmitted bool
}

func (rs rawStory) Clean() (ScrapedStory, error) {
	result := ScrapedStory{}
	story := &result.Story

	// parse id
	{
		id, err := strconv.Atoi(rs.ID)
		if err != nil {
			return result, errors.Wrapf(err, "parse story id %s", rs.ID)
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
				return result, errors.Wrapf(err, "parse story score %s", rs.Score)
			}
		}
		// if there is no upvotes field, it is a job
		// we want to treat these as stories because they get ranked
	}

	// parse age (submission time)
	{
		submissionTime, err := time.Parse("2006-01-02T15:04:05", rs.Age)
		if err != nil {
			return result, errors.Wrapf(err, "parse submission time %s", rs.Age)
		}
		story.SubmissionTime = submissionTime.Unix()

		// this will be something like "1 minute ago" or "3 hours ago"
		if fs := strings.Fields(rs.RelativeAge); len(fs) > 1 {
			var s int64
			underOneHour := false
			if strings.HasPrefix(fs[1], "minute") { // "minute" or "minutes"
				underOneHour = true
				//				fmt.Println("Got relative age", rs.RelativeAge)
				m, err := strconv.Atoi(fs[0])
				if err != nil {
					return result, errors.Wrapf(err, "parse relative age %s", rs.RelativeAge)
				}
				s = time.Now().Unix() - int64(m*60)

			} else if strings.HasPrefix(fs[1], "hour") {
				//				fmt.Println("Got relative age", rs.RelativeAge)
				m, err := strconv.Atoi(fs[0])
				if err != nil {
					return result, errors.Wrapf(err, "parse relative age %s", rs.RelativeAge)
				}

				// give it an extra hours, first because the relative age always rounds down
				// (3h50m ago is shown as 3 hours ago), and
				// second because it it is better to assume an older submission time
				// and thereby not give them the full boost they have earned, then to over
				// estimate and give them too much of a boost. Add another couple of minutes
				// because
				s = time.Now().Unix() - int64(m*3600) - 3600
			}

			if s != 0 {

				// The relative submission time seems to be off by about a minute less
				// than it should be (based on the current time and the submission time). This is
				// partly because of latency in our crawl and maybe partly something internal to HN
				s -= 60

				// fmt.Println("Submission time vs implied submission time", story.ID, rs.RelativeAge, s, story.SubmissionTime, s-story.SubmissionTime)
				// If there is more than an hour discrepancy, it indicates that this story has
				// been resubmitted. So use the new estimated time
				//				fmt.Println("Implied submission time", s-story.SubmissionTime)
				if s-story.SubmissionTime > 3600 {
					if underOneHour {
						// Only update the submission time based on the relative time if it is less than 1 hour
						// because then we have granularity of 1 minute, and
						fmt.Println("Found resubmitted story less than one hour old. Submission time vs implied submission time", story.ID, rs.RelativeAge, s-story.SubmissionTime)
						story.SubmissionTime = s
						result.Resubmitted = true
					}
				}
			}
		}

		// parse rank. we know the rank because of the order it appears in.
		// we just use this to do an integrity check later.
		{
			tRank := strings.Trim(rs.Rank, ".")
			var err error
			result.Rank, err = strconv.Atoi(tRank)
			if err != nil || result.Rank == 0 {
				return result, errors.Wrapf(err, "parse rank %s", rs.Rank)
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
					return result, errors.Wrapf(err, "parse comments %s", commentString)
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

		return result, nil
	}
}

func (app app) newScraper(resultCh chan ScrapedStory, errCh chan error) *colly.Collector {
	// func (app app) newScraper() *colly.Collector {
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
		err = errors.Wrapf(err, "Failed to parse URL %s", r.Request.URL)
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
			c := app.newScraper(resultCh, errCh)
			u := url
			if page > 1 {
				u = url + "?p=" + strconv.Itoa(page)
			}
			err := c.Visit(u)
			if err != nil {
				errCh <- err
			}
		}(p)
	}
	wg.Wait()
	close(resultCh)
	close(errCh)
}
