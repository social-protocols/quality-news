package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
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

func (d frontPageData) IsNewPage() bool {
	return d.Ranking == "new"
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

var defaultFrontPageParams = FrontPageParams{2.2956, 5.0, 1.4, 2.5}

const frontPageSQL = `
	with parameters as (select %f as priorWeight, %f as overallPriorWeight, %f as gravity, %f as penaltyWeight)
	, rankedStories as (
		select
			*
			, timestamp as OriginalSubmissionTime
			, (cumulativeUpvotes + priorWeight)/(cumulativeExpectedUpvotes + priorWeight) as quality
			, cast((sampleTime-submissionTime) as real)/3600 as ageHours
	  from stories
	  join dataset using(id)
	  join parameters
	  where sampleTime = (select max(sampleTime) from dataset)
      and score >= 3
	)
	, unadjustedRanks as (
		select
			*
			, dense_rank() over(order by %s)  as unadjustedRank
		from rankedStories
	)
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
		, dense_rank() over(order by unadjustedRank + penalty*penaltyWeight) as qnRank
		, cast((sampleTime-submissionTime)/3600 as real) as ageHours
	from unadjustedRanks
	order by %s
	limit 90;
`

const newPageSQL = `
	with parameters as (select %f as priorWeight, %f as overallPriorWeight)
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
	from dataset join stories using (id) join parameters
	where sampleTime = (select max(sampleTime) from dataset)
	order by newRank nulls last
	limit 90;
`

var statements map[string]*sql.Stmt

func (app app) serveFrontPage(r *http.Request, w http.ResponseWriter, ranking string, p FrontPageParams) error {
	d, err := app.getFrontPageData(r.Context(), ranking, p)
	if err != nil {
		return errors.Wrap(err, "getFrontPageData")
	}

	if err = templates.ExecuteTemplate(w, "index.html.tmpl", d); err != nil {
		return errors.Wrap(err, "executing front page template")
	}

	return nil
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

		var sql string
		if ranking == "new" {
			sql = fmt.Sprintf(newPageSQL, priorWeight, overallPriorWeight)
		} else {
			orderBy := "qnRank nulls last"
			if ranking == "hntop" {
				orderBy = "topRank nulls last"
			} else if ranking == "new" {
				orderBy = "newRank nulls last"
			}
			sql = fmt.Sprintf(frontPageSQL, priorWeight, overallPriorWeight, gravity, penaltyWeight, qnRankFormulaSQL, orderBy)
		}

		s, err = ndb.db.Prepare(sql)
		if err != nil {
			return stories, errors.Wrap(err, "preparing SQL")
		}

		if params == defaultFrontPageParams {
			statements[ranking] = s
		}
	} else {
		s = statements[ranking]
	}

	rows, err := s.QueryContext(ctx)
	if err != nil {
		return stories, errors.Wrap(err, "executing SQL")
	}
	defer rows.Close()

	for rows.Next() {

		var s Story

		var ageHours int
		err = rows.Scan(&s.ID, &s.By, &s.Title, &s.URL, &s.SubmissionTime, &s.OriginalSubmissionTime, &s.AgeApprox, &s.Score, &s.Comments, &s.Quality, &s.Penalty, &s.TopRank, &s.QNRank, &ageHours)

		// Don't show QN rank if we are on the QN page and vice versa
		switch ranking {
		case "quality":
			s.QNRank = sql.NullInt32{Int32: 0, Valid: false}
		case "hntop":
			s.TopRank = sql.NullInt32{Int32: 0, Valid: false}
		}

		// Don't show QN rank if it is greater than 90
		if s.QNRank.Valid && s.QNRank.Int32 > 90 {
			s.QNRank = sql.NullInt32{Int32: 0, Valid: false}
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
