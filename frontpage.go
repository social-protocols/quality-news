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
	PositionsJSONData      any
	UserID sql.NullInt64
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

func (d frontPageData) IsBestPage() bool {
	return d.Ranking == "best"
}

func (d frontPageData) IsBestUpvoteRatePage() bool {
	return d.Ranking == "best-upvoterate"
}

func (d frontPageData) IsAskPage() bool {
	return d.Ranking == "ask"
}

func (d frontPageData) IsShowPage() bool {
	return d.Ranking == "show"
}

func (d frontPageData) IsRawPage() bool {
	return d.Ranking == "raw"
}

func (d frontPageData) IsAboutPage() bool {
	return false
}

func (d frontPageData) IsDeltaPage() bool {
	return d.Ranking == "highdelta" || d.Ranking == "lowdelta"
}

func (d frontPageData) IsScorePage() bool {
	return false
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

func (d frontPageData) SampleTimeString() string {
	if d.Params.PastTime == 0 {
		return "now"
	} else {
		return time.Unix(d.Params.PastTime, 0).Format(time.RFC3339)
	}
}

type FrontPageParams struct {
	PriorWeight        float64
	OverallPriorWeight float64
	Gravity            float64
	PenaltyWeight      float64
	PastTime           int64
}

func (p FrontPageParams) String() string {
	return fmt.Sprintf("%#v", p)
}

var defaultFrontPageParams = FrontPageParams{priorWeight, 5.0, 1.4, 2.5, 0}

const frontPageSQL = `
	with parameters as (select %f as priorWeight, %f as overallPriorWeight, %f as gravity, %f as penaltyWeight, %f as fatigueFactor, %d as pastTime)
	, rankedStories as (
		select
			*
			, timestamp as OriginalSubmissionTime
			, (cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight) as quality
	  from stories
	  join dataset using(id)
	  join parameters
	  where sampleTime = case when pastTime > 0 then pastTime else (select max(sampleTime) from dataset) end
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
		, unixepoch() - sampleTime + coalesce(ageApprox, sampleTime - submissionTime) ageApprox
		, score
		, descendants
		, (cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight) as quality
		, penalty
		, topRank
		, dense_rank() over(order by unadjustedRank + penalty*penaltyWeight) as qnRank
		, rawRank
		, flagged
		, dupe
		, job
	from unadjustedRanks
	order by %s
	limit 90;
`

const hnPageSQL = `
	with parameters as (select %f as priorWeight, %f as overallPriorWeight, %f as gravity, %f as fatigueFactor, %d as pastTime)
	select
		id
		, by
		, title
		, url
		, submissionTime
		, timestamp as OriginalSubmissionTime
		, unixepoch() - sampleTime + coalesce(ageApprox, sampleTime - submissionTime) ageApprox
		, score
		, descendants
		, (cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight) as quality
		, penalty
		, topRank
		, qnRank
		, rawRank
		, flagged
		, dupe
		, job
	from dataset join stories using (id) join parameters
	where sampleTime = case when pastTime > 0 then pastTime else (select max(sampleTime) from dataset) end
	order by %s
	limit 90;
`

var statements map[string]*sql.Stmt

func (app app) serveFrontPage(r *http.Request, w http.ResponseWriter, ranking string, p FrontPageParams) error {

	userID := app.getUserID(r)

	d, err := app.getFrontPageData(r.Context(), ranking, p, userID)
	if err != nil {
		return errors.Wrap(err, "getFrontPageData")
	}

	if err = templates.ExecuteTemplate(w, "index.html.tmpl", d); err != nil {
		return errors.Wrap(err, "executing front page template")
	}

	return nil
}

func (app app) getFrontPageData(ctx context.Context, ranking string, params FrontPageParams, userID sql.NullInt64) (frontPageData, error) {
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
		weightedAverageQuality += expectedUpvoteShare(0, zeroBasedRank+1) * s.UpvoteRate
		totalUpvotes += s.Score - 1
	}

	var positions any = []any{}

	if userID.Valid {
		storyIDs := make([]int, len(stories))
		upvoteRates := map[int]float64{}
		for i, s := range stories {
			storyIDs[i] = s.ID
			upvoteRates[s.ID] = s.UpvoteRate
		}

		ps, err := app.getOpenPositions(ctx, userID.Int64, storyIDs)
		if err != nil {
			return frontPageData{}, errors.Wrap(err, "positions")
		}

		positions = mapSlice(
			ps,
			func(p Position) []any {
				if p.Exited() {
					return []any{p.StoryID, p.Direction, upvoteRates[p.StoryID], p.EntryUpvoteRate, p.ExitUpvoteRate}
				}
				return []any{p.StoryID, p.Direction, upvoteRates[p.StoryID], p.EntryUpvoteRate, nil}
			},
		)
	}

	d := frontPageData{
		stories,
		float64(totalAgeSeconds) / float64(nStories),
		weightedAverageQuality,
		float64(totalUpvotes) / float64(nStories),
		ranking,
		params,
		positions,
		userID,
	}

	return d, nil
}

func orderByStatement(ranking string) string {
	switch ranking {
	case "quality":
		return "qnRank nulls last"
	case "hntop":
		return "topRank nulls last"
	case "unadjusted":
		return hnRankFormulaSQL
	case "lowdelta":
		return "case when rawRank is null or (topRank is null and rawRank > 90) then null else ifnull(topRank,91) - rawRank end desc nulls last"
	case "highdelta":
		return "case when rawRank is null or (topRank is null and rawRank > 90) then null else ifnull(topRank,91) - rawRank end nulls last"
	case "best-upvoterate":
		return "(cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight) desc nulls last"
	default:
		return fmt.Sprintf("%sRank nulls last", ranking)
	}
}

func getFrontPageStories(ctx context.Context, ndb newsDatabase, ranking string, params FrontPageParams) (stories []Story, err error) {
	if statements == nil {
		statements = make(map[string]*sql.Stmt)
	}

	var s *sql.Stmt

	// Prepare statement if it hasn't already been prepared or if we are using
	// custom parameters
	if statements[ranking] == nil || params != defaultFrontPageParams {

		var sql string
		orderBy := orderByStatement(ranking)
		if ranking == "quality" {
			sql = fmt.Sprintf(frontPageSQL, params.PriorWeight, params.OverallPriorWeight, params.Gravity, params.PenaltyWeight, fatigueFactor, params.PastTime, qnRankFormulaSQL, orderBy)
		} else {
			sql = fmt.Sprintf(hnPageSQL, params.PriorWeight, params.OverallPriorWeight, params.Gravity, fatigueFactor, params.PastTime, orderBy)
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

		err = rows.Scan(&s.ID, &s.By, &s.Title, &s.URL, &s.SubmissionTime, &s.OriginalSubmissionTime, &s.AgeApprox, &s.Score, &s.Comments, &s.UpvoteRate, &s.Penalty, &s.TopRank, &s.QNRank, &s.RawRank, &s.Flagged, &s.Dupe, &s.Job)

		if ranking == "hntop" {
			s.IsHNTopPage = true
		}
		if ranking == "highdelta" || ranking == "lowdelta" {
			s.IsDeltaPage = true
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
