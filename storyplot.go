package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

func storyplot(ndb newsDatabase, storyID int) {
	// https://github.com/gonum/plot/wiki/Example-plots

	p := plot.New()

	p.Title.Text = fmt.Sprintf("Story %d", storyID)
	p.X.Label.Text = "Age [h]"

	upvotes, topRanks := storyDatapoints(ndb, storyID)

	err := plotutil.AddLinePoints(p,
		"Upvotes", upvotes,
		"TopRank", topRanks)
	if err != nil {
		panic(err)
	}

	// Save the plot to a PNG file.
	if err := p.Save(8*vg.Inch, 6*vg.Inch, "storyplot.png"); err != nil {
		panic(err)
	}
}

// randomPoints returns some random x, y points.
func randomPoints(n int) plotter.XYs {
	pts := make(plotter.XYs, n)
	for i := range pts {
		if i == 0 {
			pts[i].X = rand.Float64()
		} else {
			pts[i].X = pts[i-1].X + rand.Float64()
		}
		pts[i].Y = pts[i].X + 10*rand.Float64()
	}
	return pts
}

func storyDatapoints(ndb newsDatabase, storyID int) (plotter.XYs, plotter.XYs) {

	var n int
	if err := ndb.db.QueryRow("select count(*) from dataset where id = ?", storyID).Scan(&n); err != nil {
		log.Fatal(err)
	}

	var submissionTime int64
	if err := ndb.db.QueryRow("select submissionTime from dataset where id = ? limit 1", storyID).Scan(&submissionTime); err != nil {
		log.Fatal(err)
	}

	upvotes := make(plotter.XYs, n)
	topRanks := make(plotter.XYs, n)

	rows, err := ndb.db.Query("select sampleTime, score, topRank from dataset where id = ?", storyID)
	if err != nil {
		log.Fatal(err)
	}

	i := 0
	for rows.Next() {

		var sampleTime int64
		var score int
		var topRank sql.NullInt32

		err = rows.Scan(&sampleTime, &score, &topRank)

		if err != nil {
			log.Fatal(err)
		}
		upvotes[i].X = float64((sampleTime - submissionTime)) / 3600
		upvotes[i].Y = float64(score)

		if topRank.Valid {
			topRanks[i].X = float64((sampleTime - submissionTime)) / 3600
			topRanks[i].Y = float64(topRank.Int32)
		}
		i++
	}

	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}

	return upvotes, topRanks
}
