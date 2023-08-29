package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/johnwarden/httperror"
	"github.com/pkg/errors"
)

type voteParams struct {
	StoryID   int  `json:"storyID"`
	Direction int8 `json:"direction"`
}

type voteResponse struct {
	Error           string  `json:"error,omitempty"`
	EntryUpvoteRate float64 `json:"entryUpvoteRate"`
}

var (
	insertVoteStmt       *sql.Stmt
	getLastVoteStatement *sql.Stmt
)

func (app app) prepareVoteStatements() error {
	err := app.ndb.attachFrontpageDB()
	if err != nil {
		return errors.Wrap(err, "attachFrontpageDB")
	}

	if insertVoteStmt == nil {

		var e error
		insertVoteStmt, e = app.ndb.upvotesDB.Prepare(`
		    with parameters as (
		      select
		        ? as userID
		        , ? as storyID
		        , ? as direction
		    )
		    , openPositions as (
				select
					userID
					, storyID
					, direction
					, entryTime
				from votes
				group by userID, storyID
				having max(rowid) -- use rowID instead of entryTime because two votes can come in during the same second
			)
		    -- A vote is a duplicate only if the **latest** vote (in openPositions) for this userID and storyID
		    -- has the same direction.
		    , duplicates as (
		    	select parameters.userID, parameters.storyID, parameters.direction == ifnull(openPositions.direction,0) as duplicate
		    	from parameters 
			    left join openPositions using (userID, storyID)
		    )
		    insert into votes(userID, storyID, direction, entryUpvotes, entryExpectedUpvotes, entryTime) 
		    select 
		      parameters.userID
		      , parameters.storyID
		      , parameters.direction
		      , cumulativeUpvotes
		      , cumulativeExpectedUpvotes
		      , unixepoch()
		    from parameters
		    -- join on dataset to get latest upvoteRate
		    join dataset on 
		      id = parameters.storyID 
		      and sampleTime = ( select max(sampleTime) from dataset join parameters where id = storyID )
		    -- but don't insert a vote unless it actually changes the user's position
		    join stories using (id)
		    join duplicates
		    where 
		    	not duplicate
			    and not job 
		`)

		if e != nil {
			return errors.Wrap(e, "Preparing insertVoteStmt")
		}

	}

	if getLastVoteStatement == nil {
		var e error

		getLastVoteStatement, e = app.ndb.upvotesDB.Prepare(`
			select 
			entryUpvotes
			, entryExpectedUpvotes
			, entryTime from 
			votes
			where userID = ? and storyID = ? and direction = ?
		`)
		if e != nil {
			return errors.Wrap(e, "Preparing getLastVoteStatement")
		}
	}
	return nil
}

func (app app) vote(ctx context.Context, userID int64, storyID int, direction int8) (r float64, t int64, err error) {
	if userID < 100 {
		return 0, 0, httperror.PublicErrorf(http.StatusUnauthorized, "Can't vote for special user IDs")
	}

	err = app.prepareVoteStatements()
	if err != nil {
		return 0, 0, err
	}

	db, err := app.ndb.upvotesDBWithDataset(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "upvotesDBWithDataset")
	}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		err = errors.Wrap(e, "BeginTX")
		return
	}

	// Use the commit/rollback in a defer pattern described in:
	// https://stackoverflow.com/questions/16184238/database-sql-tx-detecting-commit-or-rollback
	defer func() {
		if err != nil {
			// https://go.dev/doc/database/execute-transactions
			// If the transaction succeeds, it will be committed before the function exits, making the deferred rollback call a no-op.
			app.logger.Debug("Rolling back transaction")
			e := tx.Rollback()
			if e != nil {
				app.logger.Error("tx.Rollback in vote", e)
			}
			return
		}
		app.logger.Debug("Commit transaction")
		err = tx.Commit() // here we are setting the return value err
		app.logger.Debug("Committed")
		if err != nil {
			return
		}
	}()

	res, err := tx.Stmt(insertVoteStmt).ExecContext(ctx, userID, storyID, direction)
	if err != nil {
		return 0, 0, errors.Wrap(err, "insertVoteStmt")
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		Debugf(app.logger, "Duplicate vote %#v, %#v", rows, e)
	} else {
		Debugf(app.logger, "Inserted vote statement %v, %d, %d", userID, storyID, direction)
	}

	row := tx.Stmt(getLastVoteStatement).QueryRowContext(ctx, userID, storyID, direction)
	var entryUpvotes int
	var entryExpectedUpvotes float64
	var entryTime int64
	err = row.Scan(&entryUpvotes, &entryExpectedUpvotes, &entryTime)
	if err != nil {
		return 0, 0, errors.Wrapf(err, "getLastVoteStatement %v %d %d", userID, storyID, direction)
	}
	entryUpvoteRate := defaultModelParams.upvoteRate(entryUpvotes, entryExpectedUpvotes)
	app.logger.Debug("Got last vote", "entryUpvoteRate", entryUpvoteRate)

	return entryUpvoteRate, entryTime, nil
}

func (app app) voteHandler() func(http.ResponseWriter, *http.Request, voteParams) error {
	return func(w http.ResponseWriter, r *http.Request, p voteParams) error {
		userID := app.getUserID(r)

		if !userID.Valid {
			return httperror.PublicErrorf(http.StatusUnauthorized, "not logged in")
		}

		app.logger.Debug("Called upvote handler")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		storyID := p.StoryID
		if storyID <= 0 {
			return fmt.Errorf("Invalid story ID %d", storyID)
		}
		Debugf(app.logger, "SToryID %d", storyID)

		direction := p.Direction
		if direction < -1 || direction > 1 {
			return fmt.Errorf("Invalid direction %d", direction)
		}

		var b []byte
		var err error
		entryUpvoteRate, _, err := app.vote(r.Context(), userID.Int64, storyID, direction)

		var response voteResponse

		if err != nil {
			app.logger.Error("Writing error response", err)
			response = voteResponse{Error: "Internal error"}
		} else {
			response = voteResponse{EntryUpvoteRate: entryUpvoteRate}
		}

		b, err = json.Marshal(response)
		if err != nil {
			_, _ = w.Write([]byte(`{error: "internal error marshaling response"}`))
			return errors.Wrap(err, "Marshaling voteResponse")
		}
		_, err = w.Write(b)
		return errors.Wrap(err, "writing HTTP response")
	}
}
