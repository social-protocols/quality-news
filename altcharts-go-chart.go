package main

import (
	"database/sql"
	"math"
	"net/http"

	"github.com/johnwarden/httperror/v2"
	"github.com/pkg/errors"
	"github.com/wcharczuk/go-chart/v2"         // exposes "chart"
	"github.com/wcharczuk/go-chart/v2/drawing" // exposes "drawing"
)

type chartParams struct{}

var transparent = drawing.ColorWhite.WithAlpha(0)

func (app app) altChartsGoChart() httperror.XHandlerFunc[chartParams] {
	return func(w http.ResponseWriter, _ *http.Request, p chartParams) error {
		xAxis, topRanks, qnRanks, err := rankDatapointsGoChart(app.ndb, 33382056)
		if err != nil {
			return errors.Wrap(err, "rankDataPoints")
		}

		// colorPallate := chart.colorPallate{

		// }

		graph := chart.Chart{
			Title: "HN vs QN Rank",

			Background: chart.Style{
				FillColor: transparent,
				// FillColor:   drawing.ColorRed.WithAlpha(64),
			},
			Canvas: chart.Style{
				FillColor: transparent,
				// FillColor:   drawing.ColorRed.WithAlpha(64),
			},
			Series: []chart.Series{
				chart.ContinuousSeries{
					XValues: xAxis,
					YValues: topRanks,
					Style: chart.Style{
						StrokeColor: drawing.ColorFromHex("FF6600"),
						StrokeWidth: 3,
					},
				},
				chart.ContinuousSeries{
					XValues: xAxis,
					YValues: qnRanks,
					Style: chart.Style{
						StrokeColor: drawing.ColorFromHex("0089F4"),
						StrokeWidth: 3,
					},
				},
			},
			XAxis: chart.XAxis{
				Name: "Age [h]",
				// Ticks: []chart.Tick{
				// 	{Value: .25, Label: "15m"},
				// 	{Value: .5, Label: "30m"},
				// 	{Value: .75, Label: "45m"},
				// 	{Value: 1, Label: "1hr"},
				// 	{Value: 2, Label: "2hr"},
				// 	{Value: 4, Label: "3hr"},
				// 	{Value: 8, Label: "8hr"},
    //                 {Value: 16, Label: "16hr"},
    //                 {Value: 32, Label: "24hr"},
    //                 {Value: 48, Label: "48hr"},
    //                 {Value: 72, Label: "72hr"},
				// },
                Range: &chart.ContinuousRange{
                    Max:        72,
                    Min:        0,
                },
			},
			YAxis: chart.YAxis{
				Name:      "Rank",
				Ascending: true,
				Range: &chart.ContinuousRange{
					Max:        math.Log2(1),
					Min:        math.Log2(91),
					Descending: true,
				},

				// ValueFormatter: func(v any) string {
				// 	return "value"
				// },
				Ticks: []chart.Tick{
					{Value: math.Log2(1), Label: "1"},
					{Value: math.Log2(2), Label: "2"},
					{Value: math.Log2(4), Label: "4"},
					{Value: math.Log2(8), Label: "8"},
					{Value: math.Log2(16), Label: "16"},
					{Value: math.Log2(32), Label: "32"},
					{Value: math.Log2(64), Label: "64"},
					{Value: math.Log2(91), Label: "> 90"},
				},
			},
		}

		// graph2 := chart.Chart{
		// 	Series: []chart.Series{
		// 		chart.ContinuousSeries{
		// 			Style: chart.Style{
		// 				StrokeColor: chart.GetDefaultColor(0).WithAlpha(64),
		// 				FillColor:   chart.GetDefaultColor(0).WithAlpha(64),
		// 			},
		// 			XValues: []float64{1.0, 2.0, 3.0, 4.0, 5.0},
		// 			YValues: []float64{1.0, 2.0, 3.0, 4.0, 5.0},
		// 		},
		// 	},
		// }

		// buffer := bytes.NewBuffer([]byte{})
		err = graph.Render(chart.PNG, w)
		if err != nil {
			return err
		}

		// line.Render(w)

		return nil
	}
}

func rankDatapointsGoChart(ndb newsDatabase, storyID int) ([]float64, []float64, []float64, error) {
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

	topRanks := make([]float64, n)
	qnRanks := make([]float64, n)
	xAxis := make([]float64, n)

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
			topRanks[i] = math.Log2(float64(topRank.Int32))
		} else {
			topRanks[i] = math.Log2(float64(91))
		}

		if qnRank.Valid {
			qnRanks[i] = math.Log2(float64(qnRank.Int32))
		} else {
			qnRanks[i] = math.Log2(float64(91))
		}

		xAxis[i] = float64(sampleTime-submissionTime) / 3600 // humanize.Time(time.Unix(sampleTime, 0))

		// return humanize.Time(time.Unix(time.Now().Unix()-int64(d.AverageAge), 0))

		// topRanks[i].X = float64((sampleTime - submissionTime)) / 3600
		// topRanks[i].Y = 91
		// if topRank.Valid {
		//  topRanks[i].Y = float64(topRank.Int32)
		// }

		// qnRanks[i].X = float64((sampleTime - submissionTime)) / 3600
		// qnRanks[i].Y = 91
		// if qnRank.Valid {
		//  qnRanks[i].Y = float64(qnRank.Int32)
		// }
		i++
	}

	err = rows.Err()

	return xAxis, topRanks, qnRanks, errors.Wrap(err, "rows.Err")
}
