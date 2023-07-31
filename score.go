package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/johnwarden/httperror"
	"github.com/pkg/errors"
)

type ScorePageData struct {
	DefaultPageHeaderData
	Positions []Position
	Score         float64
	ScorePlotData [][]any
}

func (d ScorePageData) IsScorePage() bool {
	return true
}

func (p ScorePageData) ScoreString() string {
	return fmt.Sprintf("%.2f", p.Score)
}

type ScorePageParams struct {
	UserID sql.NullInt64
}

func (app app) scoreHandler() func(http.ResponseWriter, *http.Request, ScorePageParams) error {

	return func(w http.ResponseWriter, r *http.Request, p ScorePageParams) error {
		nullUserID := p.UserID
		if !nullUserID.Valid {

			nullUserID = app.getUserID(r)

			if !nullUserID.Valid {
				return httperror.PublicErrorf(http.StatusUnauthorized, "not logged in")
				// 	userID = 1
			}
		}

		userID := int(nullUserID.Int64)

		positions, err := app.getAllPositions(r.Context(), userID)
		if err != nil {
			return errors.Wrap(err, "getAllPositions")
		}

		var score float64
		for i, p := range positions {
			score += p.Gain()
			positions[i].RunningScore = score
		}

		n := len(positions)
		for i := range positions {
			positions[i].RunningScore = score - positions[i].RunningScore + positions[i].Gain()
			positions[i].Label = intToAlphaLabel(n - i - 1)
		}

		scorePlotData := make([][]any, n, n)
		for i, p := range positions {
			scorePlotData[n-i-1] = []any{
				p.EntryTime, p.RunningScore, fmt.Sprintf("%d", p.PositionID),
			}
		}

		pageSize := 1000
		if n > pageSize {
			n = pageSize
		}

		d := ScorePageData{DefaultPageHeaderData{nullUserID}, positions[0:n], score, scorePlotData}

		if err = templates.ExecuteTemplate(w, "score.html.tmpl", d); err != nil {
			return errors.Wrap(err, "executing score template")
		}

		return nil	
	}
}

// convert an integer into an alpha-numerical label starting with A through Z, then continuing AA, AB, etc.

func intToAlphaLabel(i int) string {
	r := make([]byte, 0, 1)

	// result := ""
	n := 0
	for {
		digit := i % 26
		letter := 'A' + digit
		// result = string(letter) + result

		r = append(r, byte(letter))

		i -= digit
		if i == 0 {
			break
		}
		i /= 26
		i -= 1
		n++
	}

	n = len(r)
	for i := 0; i < n/2; i++ {
		j := n - i - 1

		r[i], r[j] = r[j], r[i]
	}

	return string(r)
}

func (app app) getAllPositions(ctx context.Context, userID int) ([]Position, error) {
	positions := make([]Position, 0)

	// TODO: only select votes relevant to the stories on the page
	// -- %f as oldpriorWeight
	// -- , 0.6 as priorWeight
	positionsStatement, err := app.ndb.upvotesDB.Prepare(
		fmt.Sprintf(`
		with params as (
			select 
			  %f as priorWeight
			  , %f as fatigueFactor
			)
		select
			userID
			, storyID
			, positionID
			, direction
			, entryTime
			, positions.entryUpvotes
			, positions.entryExpectedUpvotes
			, positions.entryUpvoteRate
			, (entryUpvotes + direction + priorWeight)/((1-exp(-fatigueFactor*entryExpectedUpvotes))/fatigueFactor + priorWeight) postEntryUpvoteRate
			, exitTime
			, exitUpvoteRate
			, cumulativeUpvotes
			, cumulativeExpectedUpvotes
			, (cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight) currentUpvoteRate
			, (cumulativeUpvotes - entryUpvotes + priorWeight)/((1-exp(-fatigueFactor*(cumulativeExpectedUpvotes-entryExpectedUpvotes)))/fatigueFactor + priorWeight) subsequentUpvoteRate
			, title
			, url
			, by
			, unixepoch() - sampleTime + coalesce(ageApprox, sampleTime - submissionTime) ageApprox
			, score
			, descendants as comments
			from positions join params
			join dataset on 
			  positions.storyID = id
			  and userID = ?
			join stories using (id)
			group by positionID
			having max(dataset.sampleTime)
			order by entryTime desc
		`, defaultFrontPageParams.PriorWeight, fatigueFactor))
	if err != nil {
		LogFatal(app.logger, "Preparing positionsStatement", err)
	}

	rows, err := positionsStatement.QueryContext(ctx, userID)
	if err != nil {
		return positions, errors.Wrap(err, "positionsStatement")
	}
	defer rows.Close()

	for rows.Next() {
		// var storyID int
		// var direction int8
		// var exitTime sql.NullInt64
		// var upvoteRate, endingUpvoteRate float64

		var p Position
		var upvotes int
		var expectedUpvotes float64
		// var subsequentUpvoteRate float

		err := rows.Scan(
			&userID,
			&p.StoryID,
			&p.PositionID,
			&p.Direction,
			&p.EntryTime,
			&p.EntryUpvotes,
			&p.EntryExpectedUpvotes,
			&p.EntryUpvoteRate,
			&p.PostEntryUpvoteRate,
			&p.ExitTime,
			&p.ExitUpvoteRate,
			&upvotes,
			&expectedUpvotes,
			&p.CurrentUpvoteRate,
			&p.SubsequentUpvoteRate,
			&p.Story.Title,
			&p.Story.URL,
			&p.Story.By,
			&p.Story.AgeApprox,
			&p.Story.Score,
			&p.Story.Comments)
		if err != nil {
			return positions, errors.Wrap(err, "scanning positions")
		}

		p.Story.UpvoteRate = p.CurrentUpvoteRate
		p.Story.ID = p.StoryID

		positions = append(positions, p)
	}

	return positions, nil
}

func (app app) getOpenPositions(ctx context.Context, userID int64, storyIDs []int) ([]Position, error) {
	positions := make([]Position, 0)

	// TODO: only select votes relevant to the stories on the page
	positionsStatement, err := app.ndb.upvotesDB.Prepare(`
    select
      storyID
      , direction
      , entryUpvoteRate
--      , (entryUpvotes + priorWeight)/((1-exp(-fatigueFactor*entryExpectedUpvotes))/fatigueFactor + priorWeight)
--      , exitTime
--    , exitUpvoteRate
    from positions
    where userID = ?
    and exitTime is null
    group by storyID
    having max(positionID)
  `)
	if err != nil {
		LogFatal(app.logger, "Preparing positionsStatement", err)
	}

	rows, err := positionsStatement.QueryContext(ctx, userID)
	if err != nil {
		return positions, errors.Wrap(err, "positionsStatement")
	}
	defer rows.Close()

	for rows.Next() {
		var storyID int
		var direction int8
		var entryUpvoteRate float64

		err := rows.Scan(&storyID, &direction, &entryUpvoteRate)
		if err != nil {
			return positions, errors.Wrap(err, "scanning voteHistory")
		}
		positions = append(positions, Position{StoryID: storyID, Direction: direction, EntryUpvoteRate: entryUpvoteRate})
	}

	return positions, nil
}
