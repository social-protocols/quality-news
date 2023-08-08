package main

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/johnwarden/httperror"
	"github.com/pkg/errors"
)

type ScorePageData struct {
	DefaultPageHeaderData
	Positions     []Position
	Score         float64
	ScorePlotData [][]any
}

func (d ScorePageData) IsScorePage() bool {
	return true
}

func (p ScorePageData) ScoreString() string {
	return fmt.Sprintf("%.2f", p.Score)
}

func (p ScorePageData) AverageScoreString() string {
	return fmt.Sprintf("%.2f", p.Score/float64(len(p.Positions)))
}

type ScorePageParams struct {
	UserID sql.NullInt64
	OptionalModelParams
	ScoringFormula string
}

func (app app) scoreHandler() func(http.ResponseWriter, *http.Request, ScorePageParams) error {
	return func(w http.ResponseWriter, r *http.Request, params ScorePageParams) error {
		nullUserID := params.UserID
		if !nullUserID.Valid {

			nullUserID = app.getUserID(r)

			if !nullUserID.Valid {
				return httperror.PublicErrorf(http.StatusUnauthorized, "not logged in")
			}
		}

		modelParams := params.OptionalModelParams.WithDefaults()

		userID := int(nullUserID.Int64)

		positions, err := app.getDetailedPositions(r.Context(), userID)
		if err != nil {
			return errors.Wrap(err, "getDetailedPositions")
		}

		var score float64
		for i, p := range positions {

			p.EntryUpvoteRate = modelParams.upvoteRate(p.EntryUpvotes, p.EntryExpectedUpvotes)
			p.CurrentUpvoteRate = modelParams.upvoteRate(p.CurrentUpvotes, p.CurrentExpectedUpvotes)
			p.Story.UpvoteRate = p.CurrentUpvoteRate

			if p.ExitUpvotes.Valid && p.ExitExpectedUpvotes.Valid {
				p.ExitUpvoteRate = sql.NullFloat64{
					Float64: modelParams.upvoteRate(int(p.ExitUpvotes.Int64), p.ExitExpectedUpvotes.Float64),
					Valid:   true,
				}
			}

			p.UserScore = UserScore(p, modelParams, params.ScoringFormula)

			score += p.UserScore
			p.RunningScore = score

			p.Story.UpvoteRate = p.UpvoteRate

			positions[i] = p
		}

		n := len(positions)
		for i := range positions {
			positions[i].RunningScore = score - positions[i].RunningScore + positions[i].UserScore
			positions[i].Label = intToAlphaLabel(n - i - 1)
		}

		scorePlotData := make([][]any, n)
		for i, p := range positions {
			scorePlotData[n-i-1] = []any{
				p.EntryTime, p.RunningScore, fmt.Sprintf("%d", p.PositionID), p.Story.Title, p.UserScoreString(), p.Direction, p.EntryUpvoteRateString(), p.CurrentUpvoteRateString(), p.ExitUpvoteRateString(),
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
