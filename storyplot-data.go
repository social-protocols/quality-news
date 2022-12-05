package main

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/johnwarden/httperror"
	"github.com/pkg/errors"
)

func (app app) ranksDataJSON() httperror.XHandlerFunc[StatsPageParams] {
	return func(w http.ResponseWriter, _ *http.Request, p StatsPageParams) error {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		ranks, err := rankDatapoints(app.ndb, p.StoryID)
		if err != nil {
			return errors.Wrap(err, "rankDataPoints")
		}

		return writeJSON(w, ranks)
	}
}

const nRanks = 6

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

	rows, err := ndb.db.Query("select sampleTime, (case when qnRank > 90 then 91 else qnRank end) as qnRank, topRank, newRank, bestRank, askRank, showRank from dataset where id = ?", storyID)
	if err != nil {
		return nil, errors.Wrap(err, "Query: select ranks")
	}

	i := 0
	for rows.Next() {
		var sampleTime int64

		var nullableRanks [nRanks]sql.NullInt32

		err = rows.Scan(&sampleTime, &nullableRanks[0], &nullableRanks[1], &nullableRanks[2], &nullableRanks[3], &nullableRanks[4], &nullableRanks[5])

		if err != nil {
			return nil, errors.Wrap(err, "rows.Scan")
		}

		ranks[i] = make([]any, nRanks+1)
		ranks[i][0] = float64(sampleTime-submissionTime) / 3600 // humanize.Time(time.Unix(sampleTime, 0))

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

func (app app) upvotesDataJSON() httperror.XHandlerFunc[StatsPageParams] {
	return func(w http.ResponseWriter, _ *http.Request, p StatsPageParams) error {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		upvotes, err := upvotesDatapoints(app.ndb, p.StoryID)
		if err != nil {
			return errors.Wrap(err, "upvotesDatapoints")
		}

		subchart := make([][]any, len(upvotes))

		for i, row := range upvotes {
			subchart[i] = []any{row[0], row[1], row[2]}
		}

		return writeJSON(w, subchart)
	}
}

func upvotesDatapoints(ndb newsDatabase, storyID int) ([][]any, error) {
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

	rows, err := ndb.db.Query("select sampleTime, cumulativeUpvotes, cumulativeExpectedUpvotes, penalty from dataset where id = ?", storyID)
	if err != nil {
		return nil, errors.Wrap(err, "Query: select upvotes")
	}

	i := 0
	for rows.Next() {
		var sampleTime int64
		var upvotes int
		var expectedUpvotes float64
		var penalty float64

		err = rows.Scan(&sampleTime, &upvotes, &expectedUpvotes, &penalty)

		if err != nil {
			return nil, errors.Wrap(err, "rows.Scan")
		}

		priorWeight := defaultFrontPageParams.PriorWeight
		upvotesData[i] = []any{
			float64(sampleTime-submissionTime) / 3600, // humanize.Time(time.Unix(sampleTime, 0))
			int32(upvotes),
			expectedUpvotes,
			(float64(upvotes) + priorWeight) / float64(expectedUpvotes+priorWeight),
			penalty,
		}
		i++
	}

	err = rows.Err()

	return upvotesData, errors.Wrap(err, "rows.Err")
}

func (app app) upvoteRateDataJSON() httperror.XHandlerFunc[StatsPageParams] {
	return func(w http.ResponseWriter, _ *http.Request, p StatsPageParams) error {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		upvotes, err := upvotesDatapoints(app.ndb, p.StoryID)
		if err != nil {
			return errors.Wrap(err, "upvotesDatapoints")
		}

		subchart := make([][]any, len(upvotes))

		for i, row := range upvotes {
			subchart[i] = []any{row[0], row[3], row[4]}
		}

		return writeJSON(w, subchart)
	}
}

func writeJSON(w http.ResponseWriter, j [][]any) error {
	b, err := json.Marshal(j)
	if err != nil {
		return errors.Wrap(err, "json.Marshal")
	}
	_, _ = w.Write([]byte(string(b)))

	return nil
}
