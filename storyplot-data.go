package main

import (
	"database/sql"
        "encoding/json"
	"fmt"
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

		b, err := json.Marshal(ranks)
		if err != nil {
			return errors.Wrap(err, "ranksDataJSON: json.Marshal")
		}
		w.Write([]byte(string(b)))

		return nil
	}
}

const nRanks = 6

func rankDatapoints(ndb newsDatabase, storyID int) ([][nRanks+1]any, error) {
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

	ranks := make([][nRanks+1]any, n)

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

		ranks[i][0] = float64(sampleTime - submissionTime) / 3600  // humanize.Time(time.Unix(sampleTime, 0))

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

		xAxis, upvotes, expectedUpvotes, _, err := upvotesDatapoints(app.ndb, p.StoryID)
		if err != nil {
			return errors.Wrap(err, "upvotesDatapoints")
		}

		_, _ = w.Write([]byte("[\n"))
		for i, age := range xAxis {
			if i%8 == 0 {
				_, _ = w.Write([]byte("\n\t"))
			}

			ageHours := float64(age) / 3600

			_, _ = w.Write([]byte(fmt.Sprintf("[%.4f,%d,%.2f]", ageHours, upvotes[i], expectedUpvotes[i])))
			if i < len(xAxis)-1 {
				_, _ = w.Write([]byte(", "))
			}
		}
		_, _ = w.Write([]byte("\n]"))

		return nil
	}
}

func upvotesDatapoints(ndb newsDatabase, storyID int) ([]int64, []int32, []float64, []float64, error) {
	var n int
	if err := ndb.db.QueryRow("select count(*) from dataset where id = ?", storyID).Scan(&n); err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "QueryRow: select count")
	}

	if n == 0 {
		return nil, nil, nil, nil, ErrStoryIDNotFound
	}

	var submissionTime int64
	if err := ndb.db.QueryRow("select timestamp from stories where id = ?", storyID).Scan(&submissionTime); err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "QueryRow: select submissionTime")
	}

	xAxis := make([]int64, n)
	upvotesData := make([]int32, n)
	expectedUpvotesData := make([]float64, n)
	upvoteRateData := make([]float64, n)

	rows, err := ndb.db.Query("select sampleTime, cumulativeUpvotes, cumulativeExpectedUpvotes from dataset where id = ?", storyID)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "Query: select upvotes")
	}

	i := 0
	for rows.Next() {
		var sampleTime int64
		var upvotes int
		var expectedUpvotes float64

		err = rows.Scan(&sampleTime, &upvotes, &expectedUpvotes)

		if err != nil {
			return nil, nil, nil, nil, errors.Wrap(err, "rows.Scan")
		}

		xAxis[i] = sampleTime - submissionTime
		upvotesData[i] = int32(upvotes)
		expectedUpvotesData[i] = expectedUpvotes

		priorWeight := defaultFrontPageParams.PriorWeight
		upvoteRateData[i] = (float64(upvotes) + priorWeight) / float64(expectedUpvotes+priorWeight)

		i++
	}

	err = rows.Err()

	return xAxis, upvotesData, expectedUpvotesData, upvoteRateData, errors.Wrap(err, "rows.Err")
}

func (app app) upvoteRateDataJSON() httperror.XHandlerFunc[StatsPageParams] {
	return func(w http.ResponseWriter, _ *http.Request, p StatsPageParams) error {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		xAxis, _, _, upvoteRates, err := upvotesDatapoints(app.ndb, p.StoryID)
		if err != nil {
			return errors.Wrap(err, "upvotesDatapoints")
		}

		_, _ = w.Write([]byte("[\n"))
		for i, age := range xAxis {
			if i%8 == 0 {
				_, _ = w.Write([]byte("\n\t"))
			}

			ageHours := float64(age) / 3600

			_, _ = w.Write([]byte(fmt.Sprintf("[%.4f,%.2f]", ageHours, upvoteRates[i])))
			if i < len(xAxis)-1 {
				_, _ = w.Write([]byte(", "))
			}
		}
		_, _ = w.Write([]byte("\n]"))

		return nil
	}
}
