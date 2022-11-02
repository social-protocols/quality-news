package main

import (
	"database/sql"
	"math/rand"
	"net/http"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
	"github.com/johnwarden/httperror/v2"
	"github.com/pkg/errors"
)

// generate random data for line chart
func generateLineItems() []opts.LineData {
	items := make([]opts.LineData, 0)
	for i := 0; i < 7; i++ {
		items = append(items, opts.LineData{Value: rand.Intn(300)})
	}
	return items
}

func (app app) altChartsGoEcharts() httperror.XHandlerFunc[chartParams] {
	return func(w http.ResponseWriter, _ *http.Request, p chartParams) error {
		xAxis, topRanks, qnRanks, err := rankDatapointsGoEcharts(app.ndb, 33382056)
		if err != nil {
			return errors.Wrap(err, "rankDataPoints")
		}

		// create a new line instance
		line := charts.NewLine()
		// set some global options like Title/Legend/ToolTip or anything else
		line.SetGlobalOptions(
			charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeWesteros, BackgroundColor: "rgba(255,255,255,0)"}),
			charts.WithTitleOpts(opts.Title{
				Title:    "HN vs QN Rank",
				Subtitle: "this is the subtitle",
			}),
			charts.WithYAxisOpts(opts.YAxis{
				Type:      "log",
				Scale:     true,
				Max:       91,
				Min:       1,
				AxisLabel: &opts.AxisLabel{Show: true, ShowMinLabel: true, ShowMaxLabel: true},
			}),
		)

		// Put data into instance
		line.SetXAxis(xAxis).
			// line.SetYAxis([]"").
			AddSeries("Top Rank", topRanks, charts.WithLineStyleOpts(opts.LineStyle{Color: "orange"})).
			AddSeries("QN Rank", qnRanks, charts.WithLineStyleOpts(opts.LineStyle{Color: "blue"})).
			// AddSeries("QN Rank", qnRanks).
			SetSeriesOptions(
				charts.WithLineChartOpts(opts.LineChart{Smooth: true}),
			)
		line.Render(w)

		return nil
	}
}

func rankDatapointsGoEcharts(ndb newsDatabase, storyID int) ([]string, []opts.LineData, []opts.LineData, error) {
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

	topRanks := make([]opts.LineData, n)
	qnRanks := make([]opts.LineData, n)
	xAxis := make([]string, n)

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
			topRanks[i].Value = float64(topRank.Int32)
		} else {
			topRanks[i].Value = float64(91)
		}

		if qnRank.Valid {
			qnRanks[i].Value = float64(topRank.Int32)
		} else {
			qnRanks[i].Value = float64(91)
		}

		// qnRanks = humanize.Time(time.Unix((sampleTime-submissionTime),0)) // humanize.Time(time.Unix(sampleTime, 0))

		xAxis[i] = humanize.Time(time.Unix(sampleTime, 0))

		// // return humanize.Time(time.Unix(time.Now().Unix()-int64(d.AverageAge), 0))

		// topRanks[i].X = float64((sampleTime - submissionTime)) / 3600
		// topRanks[i].Y = 91
		// if topRank.Valid {
		// 	topRanks[i].Y = float64(topRank.Int32)
		// }

		// qnRanks[i].X = float64((sampleTime - submissionTime)) / 3600
		// qnRanks[i].Y = 91
		// if qnRank.Valid {
		// 	qnRanks[i].Y = float64(qnRank.Int32)
		// }
		i++
	}

	err = rows.Err()

	return xAxis, topRanks, qnRanks, errors.Wrap(err, "rows.Err")
}
