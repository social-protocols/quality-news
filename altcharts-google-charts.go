package main

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/johnwarden/httperror/v2"
	"github.com/pkg/errors"
)

func (app app) ranksDataJSON() httperror.XHandlerFunc[StatsPageParams] {
	return func(w http.ResponseWriter, _ *http.Request, p StatsPageParams) error {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		xAxis, topRanks, qnRanks, err := rankDatapointsGoogleCharts(app.ndb, p.StoryID)
		if err != nil {
			return errors.Wrap(err, "rankDataPoints")
		}

		w.Write([]byte("[\n"))
		for i, age := range xAxis {
			if i%8 == 0 {
				w.Write([]byte("\n\t"))
			}

			ageHours := float64(age) / 3600

			w.Write([]byte(fmt.Sprintf("[%.2f,%d,%d]", ageHours, topRanks[i], qnRanks[i])))
			if i < len(xAxis)-1 {
				w.Write([]byte(", "))
			}
		}
		w.Write([]byte("\n]"))

		return nil
	}
}

func rankDatapointsGoogleCharts(ndb newsDatabase, storyID int) ([]int64, []int32, []int32, error) {
	var n int
	if err := ndb.db.QueryRow("select count(*) from dataset where id = ?", storyID).Scan(&n); err != nil {
		return nil, nil, nil, errors.Wrap(err, "QueryRow: select count")
	}

	if n == 0 {
		return nil, nil, nil, ErrStoryIDNotFound
	}

	var submissionTime int64
	if err := ndb.db.QueryRow("select submissionTime from dataset where id = ? limit 1", storyID).Scan(&submissionTime); err != nil {
		return nil, nil, nil, errors.Wrap(err, "QueryRow: select submissionTime")
	}

	xAxis := make([]int64, n)
	topRanks := make([]int32, n)
	qnRanks := make([]int32, n)

	rows, err := ndb.db.Query("select sampleTime, topRank, qnRank from dataset where id = ?", storyID)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "Query: select ranks")
	}

	i := 0
	for rows.Next() {
		var sampleTime int64
		var topRank sql.NullInt32
		var qnRank sql.NullInt32

		err = rows.Scan(&sampleTime, &topRank, &qnRank)

		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "rows.Scan")
		}

		if topRank.Valid {
			topRanks[i] = topRank.Int32
		} else {
			topRanks[i] = 91
		}

		if qnRank.Valid {
			qnRanks[i] = qnRank.Int32
		} else {
			qnRanks[i] = 91
		}

		xAxis[i] = sampleTime - submissionTime // humanize.Time(time.Unix(sampleTime, 0))

		i++
	}

	err = rows.Err()

	return xAxis, topRanks, qnRanks, errors.Wrap(err, "rows.Err")
}
