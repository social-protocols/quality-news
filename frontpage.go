package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/pkg/errors"
)

type frontPageData struct {
	Stories        []Story
	AverageAge     float64
	AverageQuality float64
	AverageUpvotes float64
	Ranking        string
	Params         FrontPageParams
}

func (d frontPageData) AverageAgeString() string {
	return humanize.Time(time.Unix(time.Now().Unix()-int64(d.AverageAge), 0))
}

func (d frontPageData) AverageQualityString() string {
	return fmt.Sprintf("%.2f", d.AverageQuality)
}

func (d frontPageData) AverageUpvotesString() string {
	return fmt.Sprintf("%.0f", d.AverageUpvotes)
}

func (d frontPageData) IsQualityPage() bool {
	return d.Ranking == "quality"
}

func (d frontPageData) IsHNTopPage() bool {
	return d.Ranking == "hntop"
}

func (d frontPageData) GravityString() string {
	return fmt.Sprintf("%.2f", d.Params.Gravity)
}

func (d frontPageData) PriorWeightString() string {
	return fmt.Sprintf("%.2f", d.Params.PriorWeight)
}

func (d frontPageData) OverallPriorWeightString() string {
	return fmt.Sprintf("%.2f", d.Params.OverallPriorWeight)
}

func (d frontPageData) PenaltyWeightString() string {
	return fmt.Sprintf("%.2f", d.Params.PenaltyWeight)
}

type FrontPageParams struct {
	PriorWeight        float64
	OverallPriorWeight float64
	Gravity            float64
	PenaltyWeight      float64
}

func (p FrontPageParams) String() string {
	return fmt.Sprintf("%#v", p)
}

var (
	defaultFrontPageParams = FrontPageParams{2.2956, 5.0, 1.4, 1}
	noFrontPageParams      FrontPageParams
)

const frontPageSQL = `
	with parameters as (select %f as priorWeight, %f as overallPriorWeight, %f as gravity, %f as penaltyWeight)
	select
		id
		, by
		, title
		, url
		, submissionTime
		, timestamp as OriginalSubmissionTime
		, ageApprox
		, score
		, descendants
		, (cumulativeUpvotes + priorWeight)/(cumulativeExpectedUpvotes + priorWeight) as quality 
		, penalty
		, topRank
		, qnRank
		, cast((sampleTime-submissionTime)/3600 as real) as ageHours
  from stories
  join dataset using(id)
  join parameters
  where sampleTime = (select max(sampleTime) from dataset)
  order by %s
  limit 90;
`

var frontPageTemplate = template.Must(template.ParseFS(resources, "templates/*"))

var statements map[string]*sql.Stmt

func (app app) generateAndCacheFrontPages(ctx context.Context) error {
	for _, ranking := range []string{"quality", "hntop"} {
		b, _, err := app.generateFrontPage(ctx, ranking, defaultFrontPageParams)
		if err != nil {
			return errors.Wrapf(err, "generateFrontPage for ranking '%s'", ranking)
		}

		app.generatedPagesMU.Lock()
		app.generatedPages[ranking] = b
		app.generatedPagesMU.Unlock()
	}

	return nil
}

func (app app) generateFrontPage(ctx context.Context, ranking string, params FrontPageParams) ([]byte, frontPageData, error) {
	t := time.Now()

	d, err := app.getFrontPageData(ctx, ranking, params)
	if err != nil {
		return nil, d, errors.Wrap(err, "getFrontPageData")
	}

	b, err := app.renderFrontPage(d)
	if err != nil {
		return nil, d, errors.Wrap(err, "generateFrontPageHTML")
	}

	app.logger.Info("Generated front page", "elapsed", time.Since(t), "ranking", ranking, "stories", len(d.Stories))

	generateFrontpageMetrics[ranking].UpdateDuration(t)

	return b, d, nil
}

func (app app) renderFrontPage(d frontPageData) ([]byte, error) {
	var b bytes.Buffer

	zw := gzip.NewWriter(&b)
	defer zw.Close()

	if err := frontPageTemplate.ExecuteTemplate(zw, "index.html.tmpl", d); err != nil {
		return nil, errors.Wrap(err, "executing front page template")
	}

	zw.Close()

	return b.Bytes(), nil
}

func (app app) getFrontPageData(ctx context.Context, ranking string, params FrontPageParams) (frontPageData, error) {
	logger := app.logger
	ndb := app.ndb

	var sampleTime int64 = time.Now().Unix()

	logger.Info("Fetching front page stories from DB", "ranking", ranking)

	stories, err := getFrontPageStories(ctx, ndb, ranking, params)
	if err != nil {
		return frontPageData{}, errors.Wrap(err, "getFrontPageStories")
	}

	nStories := len(stories)

	var totalAgeSeconds int64
	var weightedAverageQuality float64
	var totalUpvotes int
	for zeroBasedRank, s := range stories {
		totalAgeSeconds += (sampleTime - s.SubmissionTime)
		weightedAverageQuality += expectedUpvoteShare(0, zeroBasedRank+1) * s.Quality
		totalUpvotes += s.Score - 1
	}

	d := frontPageData{
		stories,
		float64(totalAgeSeconds) / float64(nStories),
		weightedAverageQuality,
		float64(totalUpvotes) / float64(nStories),
		ranking,
		params,
	}

	return d, nil
}

func getFrontPageStories(ctx context.Context, ndb newsDatabase, ranking string, params FrontPageParams) (stories []Story, err error) {
	gravity := params.Gravity
	overallPriorWeight := params.OverallPriorWeight
	priorWeight := params.PriorWeight
	penaltyWeight := params.PenaltyWeight

	if statements == nil {
		statements = make(map[string]*sql.Stmt)
	}

	var s *sql.Stmt

	// Prepare statement if it hasn't already been prepared or if we are using
	// custom parameters
	if statements[ranking] == nil || params != defaultFrontPageParams {

		orderBy := "qnRank nulls last"
		if ranking == "hntop" {
			orderBy = "topRank nulls last"
		} else if params != defaultFrontPageParams {
			orderBy = qnRankFormulaSQL
		}
		sql := fmt.Sprintf(frontPageSQL, priorWeight, overallPriorWeight, gravity, penaltyWeight, orderBy)

		s, err = ndb.db.Prepare(sql)
		if err != nil {
			return stories, errors.Wrap(err, "preparing front page SQL")
		}

		if params == defaultFrontPageParams {
			statements[ranking] = s
		}
	} else {
		s = statements[ranking]
	}

	rows, err := s.QueryContext(ctx)
	if err != nil {
		return stories, errors.Wrap(err, "executing front page SQL")
	}
	defer rows.Close()

	for rows.Next() {

		var s Story

		var ageHours int;
		err = rows.Scan(&s.ID, &s.By, &s.Title, &s.URL, &s.SubmissionTime, &s.OriginalSubmissionTime, &s.AgeApprox, &s.Score, &s.Comments, &s.Quality, &s.Penalty, &s.TopRank, &s.QNRank, &ageHours)

		if ranking == "quality" {
			s.QNRank = sql.NullInt32{Int32: 0, Valid: false}
		}

		if ranking == "hntop" {
			s.TopRank = sql.NullInt32{Int32: 0, Valid: false}
		}

		if err != nil {
			return stories, errors.Wrap(err, "Scanning row")
		}
		stories = append(stories, s)
	}

	err = rows.Err()
	if err != nil {
		return stories, err
	}

	return stories, nil
}
