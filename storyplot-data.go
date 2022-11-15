package main

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/johnwarden/httperror"
	"github.com/pkg/errors"
)

func (app app) ranksDataJSON() httperror.XHandlerFunc[StatsPageParams] {
	return func(w http.ResponseWriter, _ *http.Request, p StatsPageParams) error {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		xAxis, ranks, err := rankDatapoints(app.ndb, p.StoryID)
		if err != nil {
			return errors.Wrap(err, "rankDataPoints")
		}

		_, _ = w.Write([]byte("[\n"))
		for i, age := range xAxis {
			if i%5 == 0 {
				_, _ = w.Write([]byte("\n\t"))
			}

			ageHours := float64(age) / 3600

			_, _ = w.Write([]byte(fmt.Sprintf("[%.4f", ageHours)))

			for _, rank := range ranks[i] {
				_, _ = w.Write([]byte(fmt.Sprintf(",%d", rank)))
			}
			_, _ = w.Write([]byte("]"))

			if i < len(xAxis)-1 {
				_, _ = w.Write([]byte(", "))
			}
		}
		_, _ = w.Write([]byte("\n]"))

		return nil
	}
}

const nRanks = 6

func rankDatapoints(ndb newsDatabase, storyID int) ([]int64, [][nRanks]int32, error) {
	var n int
	if err := ndb.db.QueryRow("select count(*) from dataset where id = ?", storyID).Scan(&n); err != nil {
		return nil, nil, errors.Wrap(err, "QueryRow: select count")
	}

	if n == 0 {
		return nil, nil, ErrStoryIDNotFound
	}

	var submissionTime int64
	if err := ndb.db.QueryRow("select timestamp from stories where id = ?", storyID).Scan(&submissionTime); err != nil {
		return nil, nil, errors.Wrap(err, "QueryRow: select submissionTime")
	}

	xAxis := make([]int64, n)
	ranks := make([][nRanks]int32, n)

	rows, err := ndb.db.Query("select sampleTime, (case when qnRank > 90 then 91 else qnRank end) as qnRank, topRank, newRank, bestRank, askRank, showRank from dataset where id = ?", storyID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Query: select ranks")
	}

	i := 0
	for rows.Next() {
		var sampleTime int64

		var nullableRanks [nRanks]sql.NullInt32

		err = rows.Scan(&sampleTime, &nullableRanks[0], &nullableRanks[1], &nullableRanks[2], &nullableRanks[3], &nullableRanks[4], &nullableRanks[5])

		if err != nil {
			return nil, nil, errors.Wrap(err, "rows.Scan")
		}

		for j, rank := range nullableRanks {
			if rank.Valid {
				ranks[i][j] = rank.Int32
			} else {
				ranks[i][j] = 91
			}
		}

		xAxis[i] = sampleTime - submissionTime // humanize.Time(time.Unix(sampleTime, 0))

		i++
	}

	err = rows.Err()

	return xAxis, ranks, errors.Wrap(err, "rows.Err")
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
