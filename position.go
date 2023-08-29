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

	db, err := app.ndb.upvotesDBWithDataset(ctx)
	if err != nil {
		return positions, errors.Wrap(err, "upvotesDBWithDataset")
	}

	var statement *sql.Stmt
	// userIDs < 100 are pseudo-users that upvote randomly according to a strategy
	Debugf(app.logger, "Getting positions for user %d", userID)
	if userID < 100 {
		switch userID {
		case 0:
			randomNewVoterStmt, err := db.PrepareContext(ctx, fmt.Sprintf(randomVoterSQL, "new", userID))
			if err != nil {
				return positions, errors.Wrap(err, "Preparing randomNewVoterStmt")
			}
			statement = randomNewVoterStmt
		case 1:
			randomTopVoterStmt, err := db.PrepareContext(ctx, fmt.Sprintf(randomVoterSQL, "top", userID))
			if err != nil {
				return positions, errors.Wrapf(err, "Preparing randomTopVoterStmt %s", fmt.Sprintf(randomVoterSQL, "top"))
			}
			statement = randomTopVoterStmt
		case 2:
			everyStoryVoter, err := db.PrepareContext(ctx, fmt.Sprintf(everyStoryVoterSQL, userID, 1))
			if err != nil {
				return positions, errors.Wrap(err, "Preparing everyStoryVoter")
			}
			statement = everyStoryVoter
		case 3:
			everyStoryDownVoter, err := db.PrepareContext(ctx, fmt.Sprintf(everyStoryVoterSQL, userID, -1))
			if err != nil {
				return positions, errors.Wrap(err, "Preparing everyStoryVoter")
			}
			statement = everyStoryDownVoter

		default:
			return positions, httperror.PublicErrorf(http.StatusUnauthorized, "Unknown user ID")
		}

		// These special user IDs are  causing the app to freeze up in production. So disable for now.
		// return positions, httperror.PublicErrorf(http.StatusUnauthorized, "Unknown user ID")

		// var sqlFilename string
		// switch userID {
		// case 0:
		// 	sqlFilename = "random-new-voter.sql"
		// case 1:
		// 	sqlFilename = "random-top-voter.sql"
		// default:
		// 	return positions, httperror.PublicErrorf(http.StatusUnauthorized, "Unknown user ID")
		// }

		// Debugf(app.logger, "Sql filename %s", sqlFilename)

		// tx, e := db.BeginTx(ctx, nil)
		// if e != nil {
		// 	return positions, errors.Wrap(e, "BeginTX")
		// }

		// err := executeSQLFile(ctx, tx, sqlFilename)
		// if err != nil {
		// 	return positions, errors.Wrap(err, "executing "+sqlFilename)
		// }

		// err = tx.Commit()
		// if err != nil {
		// 	return positions, errors.Wrap(err, "tx.Commit in getDetailedPositions")
		// }
	} else {

		getDetailedPositionsStmt, err := db.PrepareContext(ctx, getDetailedPositionsSQL)
		if err != nil {
			return positions, errors.Wrap(err, "Preparing getDetailedPositionsStmt")
		}

		statement = getDetailedPositionsStmt
	}

	rows, err := statement.QueryContext(ctx, userID)
	if err != nil {
		return positions, errors.Wrap(err, "Getting positions")
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

	db, err := app.ndb.upvotesDBWithDataset(ctx)
	if err != nil {
		return positions, errors.Wrap(err, "upvotesDBWithDataset")
	}

	// TODO: only select votes relevant to the stories on the page
	getPositionsStatement, err := db.PrepareContext(ctx, `
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
		return positions, errors.Wrap(err, "Preparing getOpenPositions")
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

var getDetailedPositionsSQL = `
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
`

var everyStoryVoterSQL = `
with storiesToUpvote as (
  select id as storyID
    , min(sampleTime) as minSampleTime
    , min(cumulativeUpvotes) as minUpvotes
    , min(cumulativeExpectedUpvotes) as minExpectedUpvotes
  from dataset join stories using (id)
  where id > (select max(id) from stories) - 100000
  and timestamp > ( select min(sampleTime) from dataset ) -- only stories submitted since we started crawling
  and not job
  group by id
  order by id desc
  limit 1000
)
, positions as (
  select 
    %d as userID
    , storiesToUpvote.storyID
    , %d as direction
    , minSampleTime as entryTime
    , minUpvotes as entryUpvotes
    , minExpectedUPvotes as entryExpectedUpvotes
    , null as exitTime
    , null as exitUpvotes
    , null as exitExpectedUpvotes
    , row_number() over () as positionID
  from storiesToUpvote
  -- left join votes existingVotes using (storyID)
  -- where existingVotes.storyID is null
) 
` + getDetailedPositionsSQL

var randomVoterSQL = `
with randomDatapoints as (
  select 
    id, sampleTime , cumulativeUpvotes, cumulativeExpectedUpvotes
    , null as exitTime
    , null as exitUpvotes
    , null as exitExpectedUpvotes
    , row_number() over () as i
    , count() over () as nIDs
  from dataset 
  join stories using (id)
  where
  timestamp > ( select min(sampleTime) from dataset ) -- only stories submitted since we started crawling
  and sampleTime > ( select max(sampleTime) from dataset ) - 24 * 60 * 60
  and %sRank is not null 
  and not job
), 
 limits as (
  select abs(random()) %% ( nIds / 1000 ) as n
  from randomDatapoints
  where i = 1
)
, storiesToUpvote as (
  select id as storyID
    , min(sampleTime) as minSampleTime
    , min(cumulativeUpvotes) as minUpvotes
    , min(cumulativeExpectedUpvotes) as minExpectedUpvotes
  from randomDatapoints join limits
  where
   ( i ) %% (nIDs / 1000) = n
  group by id
  order by sampleTime
  limit 1000
)
, positions as (
  select 
    %d as userID
    , storiesToUpvote.storyID
    , 1 as direction
    , minSampleTime as entryTime
    , minUpvotes as entryUpvotes
    , minExpectedUPvotes as entryExpectedUpvotes
    , null as exitTime
    , null as exitUpvotes
    , null as exitExpectedUpvotes
    , row_number() over () as positionID
  from storiesToUpvote
  -- left join votes existingVotes using (storyID)
  -- where existingVotes.storyID is null
) ` + getDetailedPositionsSQL
