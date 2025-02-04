package main

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/pkg/errors"
)

type frontPageData struct {
	Stories           []StoryTemplateData
	AverageAge        float64
	AverageQuality    float64
	AverageUpvotes    float64
	Ranking           string
	Params            FrontPageParams
	PositionsJSONData any
	UserID            sql.NullInt64
	PageFlags
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

func (d frontPageData) IsFairPage() bool {
	return d.Ranking == "fair"
}

func (d frontPageData) IsUpvoteratePage() bool {
	return d.Ranking == "upvoterate"
}
func (d frontPageData) IsBestUpvoteratePage() bool {
	return d.Ranking == "best-upvoterate"
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

func (d frontPageData) IsAlgorithmsPage() bool {
	return false
}

func (d frontPageData) IsPenaltiesPage() bool {
	return d.Ranking == "penalties"
}

func (d frontPageData) IsBoostsPage() bool {
	return d.Ranking == "boosts"
}

func (d frontPageData) IsResubmissionsPage() bool {
	return d.Ranking == "resubmissions"
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
	ModelParams
	// PriorWeight        float64
	OverallPriorWeight float64
	Gravity            float64
	PenaltyWeight      float64
	PastTime           int64
}

type OptionalFrontPageParams struct {
	OptionalModelParams
	// PriorWeight        float64
	OverallPriorWeight sql.NullFloat64
	Gravity            sql.NullFloat64
	PenaltyWeight      sql.NullFloat64
	PastTime           sql.NullInt64
}

func (p OptionalFrontPageParams) WithDefaults() FrontPageParams {
	var results FrontPageParams

	results.ModelParams = p.OptionalModelParams.WithDefaults()

	if p.Gravity.Valid {
		results.Gravity = p.Gravity.Float64
	} else {
		results.Gravity = defaultFrontPageParams.Gravity
	}

	if p.OverallPriorWeight.Valid {
		results.OverallPriorWeight = p.OverallPriorWeight.Float64
	} else {
		results.OverallPriorWeight = defaultFrontPageParams.OverallPriorWeight
	}

	if p.PriorWeight.Valid {
		results.PriorWeight = p.PriorWeight.Float64
	} else {
		results.PriorWeight = defaultFrontPageParams.PriorWeight
	}

	if p.FatigueFactor.Valid {
		results.FatigueFactor = p.FatigueFactor.Float64
	} else {
		results.FatigueFactor = defaultFrontPageParams.FatigueFactor
	}

	if p.PenaltyWeight.Valid {
		results.PenaltyWeight = p.PenaltyWeight.Float64
	} else {
		results.PenaltyWeight = defaultFrontPageParams.PenaltyWeight
	}

	if p.PastTime.Valid {
		results.PastTime = p.PastTime.Int64
	} else {
		results.PastTime = defaultFrontPageParams.PastTime
	}

	return results
}

func (p FrontPageParams) String() string {
	return fmt.Sprintf("%#v", p)
}

var defaultFrontPageParams = FrontPageParams{defaultModelParams, 5.0, 1.4, 2.5, 0}

const pageSQL = `
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
		, cumulativeUpvotes
		, cumulativeExpectedUpvotes
		-- , (cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight) as quality
		, topRank
		, qnRank
		, rawRank
		, flagged
		, dupe
		, job
	from dataset join stories using (id) join parameters
	where sampleTime = case when pastTime > 0 then pastTime else (select max(sampleTime) from dataset) end
	and (%s)
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
	ndb := app.ndb

	var sampleTime int64 = time.Now().Unix()

	stories, err := getFrontPageStories(ctx, ndb, ranking, params)
	if err != nil {
		return frontPageData{}, errors.Wrap(err, "getFrontPageStories")
	}

	pageFlags := PageFlags{
		IsHNTopPage:          ranking == "hntop",
		IsFairPage:           ranking == "fair",
		IsUpvoteratePage:     ranking == "upvoterate",
		IsBestUpvoteratePage: ranking == "best-upvoterate",
		IsStatsPage:          false,
		IsPenaltiesPage:      ranking == "penalties",
		IsBoostsPage:         ranking == "boosts",
		IsResubmissionsPage:  ranking == "resubmissions",
		IsRawPage:            ranking == "raw",
	}

	// Wrap each story in StoryTemplateData with page context
	var storyTemplates []StoryTemplateData
	for _, story := range stories {
		template := StoryTemplateData{
			Story:     story,
			PageFlags: pageFlags,
		}
		storyTemplates = append(storyTemplates, template)
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

		ps, err := app.getPositions(ctx, userID.Int64, storyIDs)
		if err != nil {
			return frontPageData{}, errors.Wrap(err, "positions")
		}

		for i := range ps {
			ps[i].EntryUpvoteRate = params.upvoteRate(ps[i].EntryUpvotes, ps[i].EntryExpectedUpvotes)
			ps[i].CurrentUpvoteRate = upvoteRates[ps[i].StoryID]

			if ps[i].ExitUpvotes.Valid && ps[i].ExitExpectedUpvotes.Valid {
				ps[i].ExitUpvoteRate = sql.NullFloat64{
					Float64: params.upvoteRate(int(ps[i].ExitUpvotes.Int64), ps[i].ExitExpectedUpvotes.Float64),
					Valid:   true,
				}
			}

		}

		positions = mapSlice(
			ps,
			func(p Position) []any {
				if p.Exited() {
					return []any{p.StoryID, p.Direction, p.CurrentUpvoteRate, p.EntryUpvoteRate, p.UserScore}
				}
				return []any{p.StoryID, p.Direction, p.CurrentUpvoteRate, p.EntryUpvoteRate, p.UserScore}
			},
		)

		for _, p := range (positions).([][]any) {
			if math.IsNaN(p[2].(float64) + p[3].(float64) + p[4].(float64)) {
				LogErrorf(app.logger, "Got bad positions record %v", p)
			}
		}

	}

	d := frontPageData{
		storyTemplates,
		float64(totalAgeSeconds) / float64(nStories),
		weightedAverageQuality,
		float64(totalUpvotes) / float64(nStories),
		ranking,
		params,
		positions,
		userID,
		pageFlags,
	}

	return d, nil
}

func orderByStatement(ranking string) string {
	switch ranking {
	case "fair":
		return `case 
			when job = 1 then topRank
			-- boosts of overRanked stories
			when topRank < rawRank and topRank < qnRank then topRank
			-- boosts of underRanked stories
			when topRank < rawRank and topRank > qnRank then qnRank
			-- penalties of overRanked stories
			when topRank > rawRank and topRank < qnRank then qnRank
			when topRank > rawRank and topRank > qnRank then topRank
		end nulls last`
	case "upvoterate":
		return "qnRank nulls last"
	case "best-upvoterate":
		return "(cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight) desc nulls last"
	case "hntop":
		return "topRank"
	case "unadjusted":
		return hnRankFormulaSQL
	case "boosts":
		// The qn ranking formula but subsituting rank delta for upvoteRate
		return "pow((sampleTime-submissionTime) * (rawRank - topRank), 0.8) / pow(cast(sampleTime-submissionTime as real)/3600+2,0.8) desc nulls last"
	case "penalties":
		// The qn ranking formula but subsituting rank delta for upvoteRate
		return "pow((sampleTime-submissionTime) * (topRank - rawRank), 0.8) / pow(cast(sampleTime-submissionTime as real)/3600+2,0.8) desc nulls last"
	case "resubmissions":
		return "submissionTime desc"
	default:
		return fmt.Sprintf("%sRank nulls last", ranking)
	}
}

func whereClause(ranking string) string {
	switch ranking {
	case "fair":
		return `
			-- we should have made penalty nullable. In any case, if it is equal to exactly 0 it means we haven't calculated a penalty
			-- because it was never in the top 90.
			topRank is not null and qnRank is not null
		 `
	case "qnrank":
		return "qnRank is not null"
	case "hntop":
		return "topRank is not null"
	case "boosts":
		return "topRank < rawRank"
	case "penalties":
		return "topRank > rawRank"
	case "resubmissions":
		return "submissionTime != timestamp"
	default:
		return "1 = 1"
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
		where := whereClause(ranking)

		sql = fmt.Sprintf(pageSQL, params.PriorWeight, params.OverallPriorWeight, params.Gravity, params.FatigueFactor, params.PastTime, where, orderBy)

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
		return stories, errors.Wrap(err, "executing front page SQL")
	}
	defer rows.Close()

	for rows.Next() {

		var s Story

		err = rows.Scan(&s.ID, &s.By, &s.Title, &s.URL, &s.SubmissionTime, &s.OriginalSubmissionTime, &s.AgeApprox, &s.Score, &s.Comments, &s.CumulativeUpvotes, &s.CumulativeExpectedUpvotes, &s.TopRank, &s.QNRank, &s.RawRank, &s.Flagged, &s.Dupe, &s.Job)

		s.UpvoteRate = params.ModelParams.upvoteRate(s.CumulativeUpvotes, float64(s.CumulativeExpectedUpvotes))

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
