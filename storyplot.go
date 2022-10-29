package main

import (
	"database/sql"
	"fmt"
	"image/color"
	"io"
	"log"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

func ranksPlot(ndb newsDatabase, storyID int) io.WriterTo {
	// https://github.com/gonum/plot/wiki/Example-plots

	hnTopRanksData, qnRanksData := rankDatapoints(ndb, storyID)

	p := plot.New()
	p.Title.Text = fmt.Sprintf("Story %d", storyID)
	p.X.Label.Text = "Age [h]"
	p.Y.Label.Text = "Rank (log)"

	topRankLine, err := plotter.NewLine(hnTopRanksData)
	if err != nil {
		panic(err)
	}
	p.Legend.Add("HN Rank", topRankLine)
	topRankLine.LineStyle.Width = vg.Points(2)
	topRankLine.LineStyle.Color = color.RGBA{R: 255, G: 102, B: 0, A: 255}

	qnRankLine, err := plotter.NewLine(qnRanksData)
	if err != nil {
		panic(err)
	}
	p.Legend.Add("QN Rank", qnRankLine)
	qnRankLine.LineStyle.Width = vg.Points(2)
	qnRankLine.LineStyle.Color = color.RGBA{R: 0, G: 137, B: 244, A: 255}

	p.Add(topRankLine, qnRankLine)
	p.Y.Scale = plot.InvertedScale{Normalizer: plot.LogScale{}}
	// Y log scale
	// p.Y.Scale = plot.LogScale{}

	writer, err := p.WriterTo(8*vg.Inch, 6*vg.Inch, "png")
	if err != nil {
		panic(err)
	}
	return writer
}

func upvotesPlot(ndb newsDatabase, storyID int) io.WriterTo {
	// https://github.com/gonum/plot/wiki/Example-plots

	upvotesData, expectedUpvotesData := upvotesDatapoints(ndb, storyID)

	p := plot.New()
	p.Title.Text = fmt.Sprintf("Story %d", storyID)
	p.X.Label.Text = "Age [h]"
	p.Y.Label.Text = "Upvotes"

	upvotesLine, err := plotter.NewLine(upvotesData)
	if err != nil {
		panic(err)
	}
	p.Legend.Add("Upvotes", upvotesLine)
	upvotesLine.LineStyle.Width = vg.Points(2)
	// upvotesLine.LineStyle.Color = color.RGBA{R: 255, G: 102, B: 0, A: 255}

	expectedUpvotesLine, err := plotter.NewLine(expectedUpvotesData)
	if err != nil {
		panic(err)
	}
	p.Legend.Add("Expected Upvotes", expectedUpvotesLine)
	expectedUpvotesLine.LineStyle.Width = vg.Points(2)
	expectedUpvotesLine.LineStyle.Color = color.RGBA{R: 0, G: 137, B: 244, A: 255}

	p.Add(upvotesLine, expectedUpvotesLine)

	writer, err := p.WriterTo(8*vg.Inch, 6*vg.Inch, "png")
	if err != nil {
		panic(err)
	}
	return writer
}

func upvoteRatePlot(ndb newsDatabase, storyID int) io.WriterTo {
	// https://github.com/gonum/plot/wiki/Example-plots

	upvoteRateData, upvoteRateBayesianData := upvoteRateDatapoints(ndb, storyID)

	p := plot.New()
	p.Title.Text = fmt.Sprintf("Story %d", storyID)
	p.X.Label.Text = "Age [h]"
	p.Y.Label.Text = "Upvotes"

	upvotesLine, err := plotter.NewLine(upvoteRateData)
	if err != nil {
		panic(err)
	}
	p.Legend.Add("Upvote Rate", upvotesLine)
	upvotesLine.LineStyle.Width = vg.Points(2)
	// upvotesLine.LineStyle.Color = color.RGBA{R: 255, G: 102, B: 0, A: 255}

	expectedUpvotesLine, err := plotter.NewLine(upvoteRateBayesianData)
	if err != nil {
		panic(err)
	}
	p.Legend.Add("Upvote Rate (Bayesian Avg)", expectedUpvotesLine)
	expectedUpvotesLine.LineStyle.Width = vg.Points(2)
	expectedUpvotesLine.LineStyle.Color = color.RGBA{R: 0, G: 137, B: 244, A: 255}

	p.Add(upvotesLine, expectedUpvotesLine)

	writer, err := p.WriterTo(8*vg.Inch, 6*vg.Inch, "png")
	if err != nil {
		panic(err)
	}
	return writer
}

func rankDatapoints(ndb newsDatabase, storyID int) (plotter.XYs, plotter.XYs) {
	var n int
	if err := ndb.db.QueryRow("select count(*) from dataset where id = ?", storyID).Scan(&n); err != nil {
		log.Fatal(err)
	}

	var submissionTime int64
	if err := ndb.db.QueryRow("select submissionTime from dataset where id = ? limit 1", storyID).Scan(&submissionTime); err != nil {
		log.Fatal(err)
	}

	topRanks := make(plotter.XYs, n)
	qnRanks := make(plotter.XYs, n)

	rows, err := ndb.db.Query("select sampleTime, topRank, qnRank from dataset where id = ?", storyID)
	if err != nil {
		log.Fatal(err)
	}

	i := 0
	for rows.Next() {
		var sampleTime int64
		var topRank sql.NullInt32
		var qnRank sql.NullInt32

		err = rows.Scan(&sampleTime, &topRank, &qnRank)

		if err != nil {
			log.Fatal(err)
		}

		topRanks[i].X = float64((sampleTime - submissionTime)) / 3600
		topRanks[i].Y = 91
		if topRank.Valid {
			topRanks[i].Y = float64(topRank.Int32)
		}

		qnRanks[i].X = float64((sampleTime - submissionTime)) / 3600
		qnRanks[i].Y = 91
		if qnRank.Valid {
			qnRanks[i].Y = float64(qnRank.Int32)
		}
		i++
	}

	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}

	return topRanks, qnRanks
}

func upvotesDatapoints(ndb newsDatabase, storyID int) (plotter.XYs, plotter.XYs) {
	var n int
	if err := ndb.db.QueryRow("select count(*) from dataset where id = ?", storyID).Scan(&n); err != nil {
		log.Fatal(err)
	}

	var submissionTime int64
	if err := ndb.db.QueryRow("select submissionTime from dataset where id = ? limit 1", storyID).Scan(&submissionTime); err != nil {
		log.Fatal(err)
	}

	upvotesData := make(plotter.XYs, n)
	expectedUpvotesData := make(plotter.XYs, n)

	rows, err := ndb.db.Query("select sampleTime, cumulativeUpvotes, cumulativeExpectedUpvotes from dataset where id = ?", storyID)
	if err != nil {
		log.Fatal(err)
	}

	i := 0
	for rows.Next() {
		var sampleTime int64
		var upvotes int
		var expectedUpvotes float64

		err = rows.Scan(&sampleTime, &upvotes, &expectedUpvotes)

		if err != nil {
			log.Fatal(err)
		}

		upvotesData[i].X = float64((sampleTime - submissionTime)) / 3600
		upvotesData[i].Y = float64(upvotes)

		expectedUpvotesData[i].X = float64((sampleTime - submissionTime)) / 3600
		expectedUpvotesData[i].Y = expectedUpvotes
		i++
	}

	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}

	return upvotesData, expectedUpvotesData
}

func upvoteRateDatapoints(ndb newsDatabase, storyID int) (plotter.XYs, plotter.XYs) {
	var n int
	if err := ndb.db.QueryRow("select count(*) from dataset where id = ?", storyID).Scan(&n); err != nil {
		log.Fatal(err)
	}

	var submissionTime int64
	if err := ndb.db.QueryRow("select submissionTime from dataset where id = ? limit 1", storyID).Scan(&submissionTime); err != nil {
		log.Fatal(err)
	}

	upvoteRateData := make(plotter.XYs, n)
	upvoteRateBayesianData := make(plotter.XYs, n)

	rows, err := ndb.db.Query("select sampleTime, cumulativeUpvotes, cumulativeExpectedUpvotes from dataset where id = ?", storyID)
	if err != nil {
		log.Fatal(err)
	}

	i := 0
	for rows.Next() {
		var sampleTime int64
		var upvotes int
		var expectedUpvotes float64

		err = rows.Scan(&sampleTime, &upvotes, &expectedUpvotes)

		if err != nil {
			log.Fatal(err)
		}

		priorWeight := defaultFrontPageParams.PriorWeight

		upvoteRateData[i].X = float64((sampleTime - submissionTime)) / 3600
		upvoteRateData[i].Y = float64(upvotes) / float64(expectedUpvotes)

		upvoteRateBayesianData[i].X = float64((sampleTime - submissionTime)) / 3600
		upvoteRateBayesianData[i].Y = (float64(upvotes) + priorWeight) / float64(expectedUpvotes+priorWeight)
		i++
	}

	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}

	return upvoteRateData, upvoteRateBayesianData
}
