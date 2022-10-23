package main

import (
	"database/sql"
	"fmt"
	"log"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

func rankComparisonPlot(ndb newsDatabase, storyID int) {
	// https://github.com/gonum/plot/wiki/Example-plots

	topRanks, qnRanks := rankDatapoints(ndb, storyID)

	p := plot.New()

	p.Title.Text = fmt.Sprintf("Story %d", storyID)
	p.X.Label.Text = "Age [h]"
	p.Y.Label.Text = "Rank"

	err := plotutil.AddLinePoints(p,
		"QN Rank", qnRanks,
		"HN Top Rank", topRanks)
	if err != nil {
		panic(err)
	}

	// Save the plot to a PNG file.
	if err := p.Save(8*vg.Inch, 6*vg.Inch, "storyplot.png"); err != nil {
		panic(err)
	}
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
