package main

import (
	"database/sql"

	"github.com/pkg/errors"
)

func maxSampleTime(ndb newsDatabase, storyID int) (int, error) {
	var n int
	err := ndb.db.QueryRow(`
			select max(sampleTime) from dataset
			where id = ?
		`, storyID).Scan(&n)

	return n, errors.Wrap(err, "QueryRow count: select max(sampleTime)")
}

func rankDatapoints(ndb newsDatabase, storyID int) ([][]any, error) {
	var n int
	if err := ndb.db.QueryRow("select count(*) from dataset where id = ?", storyID).Scan(&n); err != nil {
		return nil, errors.Wrap(err, "QueryRow: select count")
	}

	if n == 0 {
		return nil, ErrStoryIDNotFound
	}

	var submissionTime int64
	if err := ndb.db.QueryRow("select timestamp from stories where id = ?", storyID).Scan(&submissionTime); err != nil {
		return nil, errors.Wrap(err, "QueryRow: select submissionTime")
	}

	ranks := make([][]any, n)

	// rows, err := ndb.db.Query("select sampleTime, (case when qnRank > 90 then 91 else qnRank end) as qnRank, topRank, newRank, bestRank, askRank, showRank, rawRank from dataset where id = ?", storyID)
	rows, err := ndb.db.Query("select sampleTime, rawRank, topRank, newRank, bestRank, askRank, showRank from dataset where id = ?", storyID)
	if err != nil {
		return nil, errors.Wrap(err, "Query: select ranks")
	}
	defer rows.Close()

	// rawRank, top, new, bet, ask, show
	const nRanks = 6

	i := 0
	for rows.Next() {
		var sampleTime int64

		var nullableRanks [nRanks]sql.NullInt32

		err = rows.Scan(&sampleTime, &nullableRanks[0], &nullableRanks[1], &nullableRanks[2], &nullableRanks[3], &nullableRanks[4], &nullableRanks[5])

		if err != nil {
			return nil, errors.Wrap(err, "rows.Scan")
		}

		ranks[i] = make([]any, nRanks+1)
		ranks[i][0] = sampleTime

		for j, rank := range nullableRanks {
			if rank.Valid {
				ranks[i][j+1] = rank.Int32
			} else {
				ranks[i][j+1] = 91
			}
		}

		i++
	}

	err = rows.Err()

	return ranks, errors.Wrap(err, "rows.Err")
}

func upvotesDatapoints(ndb newsDatabase, storyID int, modelParams ModelParams) ([][]any, error) {
	var n int
	if err := ndb.db.QueryRow("select count(*) from dataset where id = ?", storyID).Scan(&n); err != nil {
		return nil, errors.Wrap(err, "QueryRow: select count")
	}

	if n == 0 {
		return nil, ErrStoryIDNotFound
	}

	var submissionTime int64
	if err := ndb.db.QueryRow("select timestamp from stories where id = ?", storyID).Scan(&submissionTime); err != nil {
		return nil, errors.Wrap(err, "QueryRow: select submissionTime")
	}

	upvotesData := make([][]any, n)

	rows, err := ndb.db.Query(`select sampleTime, cumulativeUpvotes, cumulativeExpectedUpvotes
 	 from dataset where id = ?`, storyID)
	if err != nil {
		return nil, errors.Wrap(err, "Query: select upvotes")
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		var sampleTime int64
		var upvotes int
		var expectedUpvotes float64
		// var upvoteRate float64

		err = rows.Scan(&sampleTime, &upvotes, &expectedUpvotes)

		if err != nil {
			return nil, errors.Wrap(err, "rows.Scan")
		}

		upvotesData[i] = []any{
			sampleTime,
			int32(upvotes),
			expectedUpvotes,
			modelParams.upvoteRate(upvotes, expectedUpvotes),
			// upvoteRate,
		}
		i++
	}

	err = rows.Err()

	return upvotesData, errors.Wrap(err, "rows.Err")
}
