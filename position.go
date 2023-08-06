package main

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/johnwarden/httperror"
	"github.com/pkg/errors"
)

type Position struct {
	UserID                 int
	StoryID                int
	PositionID             int
	Direction              int8
	EntryTime              int64
	EntryUpvotes           int
	EntryExpectedUpvotes   float64
	EntryUpvoteRate        float64
	ExitTime               sql.NullInt64
	ExitUpvotes            sql.NullInt64
	ExitExpectedUpvotes    sql.NullFloat64
	ExitUpvoteRate         sql.NullFloat64
	CurrentUpvotes         int
	CurrentExpectedUpvotes float64
	CurrentUpvoteRate      float64
	Story
	RunningScore float64
	Label        string
	UserScore    float64
}

func (p Position) VoteTypeString() string {
	switch p.Direction {
	case 1:
		return "upvoted"
	case -1:
		return "downvoted"
	default:
		panic("Invalid direction value")
	}
}

func (p Position) EntryTimeString() string {
	return humanize.Time(time.Unix(p.EntryTime, 0))
}

func (p Position) EntryUpvoteRateString() string {
	return fmt.Sprintf("%.2f", p.EntryUpvoteRate)
}

func (p Position) Exited() bool {
	return p.ExitTime.Valid
}

func (p Position) ExitTimeString() string {
	// return time.Unix(int64(s.MaxSampleTime), 0).UTC().Format("2006-01-02T15:04")
	if !p.ExitTime.Valid {
		return ""
	}
	return humanize.Time(time.Unix(p.ExitTime.Int64, 0))
}

func (p Position) ExitUpvoteRateString() string {
	if !p.Exited() {
		return ""
	}
	return fmt.Sprintf("%.2f", p.ExitUpvoteRate.Float64)
}

func (p Position) CurrentUpvoteRateString() string {
	return fmt.Sprintf("%.2f", p.CurrentUpvoteRate)
}

func (p Position) UserScoreString() string {
	gain := p.UserScore

	if math.Abs(gain) < .01 {
		return "-"
	}

	if gain > 0 {
		return fmt.Sprintf("+%.2f", gain)
	}

	return fmt.Sprintf("%.2f", gain)
}

func (p Position) IsGain() bool {
	return p.UserScore > 0
}

// Gets every position for the user, along with story details
func (app app) getDetailedPositions(ctx context.Context, userID int) ([]Position, error) {
	positions := make([]Position, 0)

	db := app.ndb.upvotesDB

	// userIDs < 100 are pseudo-users that upvote randomly according to a strategy
	Debugf(app.logger, "Getting positions for user %d", userID)
	if userID < 100 {
		// These special user IDs are  causing the app to freeze up in production. So disable for now.
		// return positions, httperror.PublicErrorf(http.StatusUnauthorized, "Unknown user ID")

		var sqlFilename string
		switch userID {
		case 0:
			sqlFilename = "random-new-voter.sql"
		case 1:
			sqlFilename = "random-top-voter.sql"
		default:
			return positions, httperror.PublicErrorf(http.StatusUnauthorized, "Unknown user ID")
		}

		Debugf(app.logger, "Sql filename %s", sqlFilename)

		tx, e := db.BeginTx(ctx, nil)
		if e != nil {
			return positions, errors.Wrap(e, "BeginTX")
		}

		err := executeSQLFile(ctx, tx, sqlFilename)
		if err != nil {
			e := tx.Rollback()
			if e != nil {
				app.logger.Error("tx.Rollback", e)
			}
			return positions, errors.Wrap(err, "executing "+sqlFilename)
		}

		err = tx.Commit()
		if err != nil {
			return positions, errors.Wrap(err, "tx.Commit")
		}
	}

	getDetailedPositionsStmt, err := db.Prepare(
		`
		select
			userID
			, storyID
			, positionID
			, direction
			, entryTime
			, entryUpvotes
			, entryExpectedUpvotes
			, exitTime
			, exitUpvotes
			, exitExpectedUpvotes
			, cumulativeUpvotes
			, cumulativeExpectedUpvotes
			, title
			, url
			, by
			, unixepoch() - sampleTime + coalesce(ageApprox, sampleTime - submissionTime) ageApprox
			, score
			, descendants as comments
			from positions 
			join dataset on 
			  positions.storyID = id
			  and userID = ?
			join stories using (id)
			group by positionID
			having max(dataset.sampleTime)
			order by entryTime desc
		`)
	if err != nil {
		LogFatal(app.logger, "Preparing getDetailedPositionsStmt", err)
	}

	rows, err := getDetailedPositionsStmt.QueryContext(ctx, userID)
	if err != nil {
		return positions, errors.Wrap(err, "getDetailedPositionsStmt")
	}
	defer rows.Close()

	for rows.Next() {
		// var storyID int
		// var direction int8
		// var exitTime sql.NullInt64
		// var upvoteRate, endingUpvoteRate float64

		var p Position

		err := rows.Scan(
			&p.UserID,
			&p.StoryID,
			&p.PositionID,
			&p.Direction,
			&p.EntryTime,
			&p.EntryUpvotes,
			&p.EntryExpectedUpvotes,
			&p.ExitTime,
			&p.ExitUpvotes,
			&p.ExitExpectedUpvotes,
			&p.CurrentUpvotes,
			&p.CurrentExpectedUpvotes,
			&p.Story.Title,
			&p.Story.URL,
			&p.Story.By,
			&p.Story.AgeApprox,
			&p.Story.Score,
			&p.Story.Comments)
		if err != nil {
			return positions, errors.Wrap(err, "scanning positions")
		}

		p.Story.ID = p.StoryID

		positions = append(positions, p)
	}

	Debugf(app.logger, "Number of Positions %d", len(positions))

	return positions, nil
}

// Gets position for the user, without details
func (app app) getPositions(ctx context.Context, userID int64, storyIDs []int) ([]Position, error) {
	positions := make([]Position, 0)

	db := app.ndb.upvotesDB

	// TODO: only select votes relevant to the stories on the page
	getPositionsStatement, err := db.Prepare(`
    select
      storyID
      , direction
      , entryUpvotes
      , entryExpectedUpvotes
      , exitUpvotes
      , exitExpectedUpvotes
    from positions
    where userID = ?
    and exitTime is null
    group by storyID
    having max(positionID)
  `)
	if err != nil {
		LogFatal(app.logger, "Preparing getOpenPositions", err)
	}

	rows, err := getPositionsStatement.QueryContext(ctx, userID)
	if err != nil {
		return positions, errors.Wrap(err, "getPositionsStatement.QuertyContext")
	}
	defer rows.Close()

	for rows.Next() {
		// var storyID int
		// var direction int8
		// var entryUpvotes int
		// var entryExpectedUpvotes float64
		var p Position

		err := rows.Scan(&p.StoryID, &p.Direction, &p.EntryUpvotes, &p.EntryExpectedUpvotes, &p.ExitUpvotes, &p.ExitExpectedUpvotes)
		if err != nil {
			return positions, errors.Wrap(err, "scanning getPositions")
		}

		positions = append(positions, p)
	}

	return positions, nil
}
