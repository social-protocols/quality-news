package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
)

type voteParams struct {
	StoryID   int  `json:"storyID"`
	Direction int8 `json:"direction"`
}

type voteResponse struct {
	Success         bool    `json:"success"`
	EntryUpvoteRate float64 `json:"entryUpvoteRate"`
}

func (app app) voteHandler() func(http.ResponseWriter, *http.Request, voteParams) error {
	fmt.Println("Creating vote handler")

	insertVoteStatement, err := app.ndb.upvotesDB.Prepare(fmt.Sprintf(`
	    with parameters as (
	      select
	        ? as userID
	        , ? as storyID
	        , ? as direction
	        , %f as priorWeight
	        , %f as fatigueFactor
	    )
	    , openPositions as (
			select
				userID
				, storyID
				, direction
--				, upvoteRate
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
	    insert into votes(userID, storyID, direction, entryUpvotes, entryExpectedUpvotes, entryUpvoteRate, entryTime) 
	    select 
	      parameters.userID
	      , parameters.storyID
	      , parameters.direction
	      , cumulativeUpvotes
	      , cumulativeExpectedUpvotes
	      , (cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight)
	      , unixepoch()
	    from parameters
	    -- join on dataset to get latest upvoteRate
	    join dataset on 
	      id = parameters.storyID 
	      and sampleTime = ( select max(sampleTime) from dataset join parameters where id = storyID )
	    -- but don't insert a vote unless it actually changes the user's position
	    join duplicates
	    where not duplicate 
	`, defaultFrontPageParams.PriorWeight, fatigueFactor))
	if err != nil {
		LogFatal(app.logger, "Preparing insertVoteStatement", err)
	}

	getLastVoteStatement, err := app.ndb.upvotesDB.Prepare(`
		select entryUpvoteRate, entryTime from votes where userID = ? and storyID = ? and direction = ?
	`)
	if err != nil {
		LogFatal(app.logger, "Preparing getLastVoteStatement", err)
	}

	return func(w http.ResponseWriter, r *http.Request, p voteParams) error {
		userID := app.getUserID(r)

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

		res, err := insertVoteStatement.ExecContext(r.Context(), userID, storyID, direction)
		if err != nil {
			return errors.Wrap(err, "insertVoteStatement")
		}
		rows, e := res.RowsAffected()
		if rows == 0 {
			Debugf(app.logger, "Duplicate vote %#v, %#v", rows, e)
		} else {
			Debugf(app.logger, "Inserted vote statement %v, %d, %d", userID, storyID, direction)
		}

		// TODO: this should be inside same transaction
		row := getLastVoteStatement.QueryRowContext(r.Context(), userID, storyID, direction)
		var entryUpvoteRate float64
		var entryTime int64
		err = row.Scan(&entryUpvoteRate, &entryTime)
		if err != nil {
			return errors.Wrapf(err, "getLastVoteStatement %v %d %d", userID, storyID, direction)
		}
		app.logger.Debug("Got last vote statement", "entryUpvoteRate", entryUpvoteRate)

		b, err := json.Marshal(voteResponse{Success: true, EntryUpvoteRate: entryUpvoteRate})
		if err != nil {
			return errors.Wrap(err, "executing vote.json template")
		}
		_, err = w.Write(b)
		return errors.Wrap(err, "writing HTTP response")
	}
}

// type Position = struct {
// 	userID     int
// 	storyID    int
// 	direction  int8
// 	upvoteRate float64
// }
