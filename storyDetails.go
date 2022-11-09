package main

import (
	"database/sql"
	"fmt"
	"time"

	humanize "github.com/dustin/go-humanize"
)

type Story struct {
	ID             int
	By             string
	Title          string
	URL            string
	SubmissionTime int64
	Upvotes        int
	Comments       int
	Quality        float64
	TopRank        sql.NullInt32
	QNRank         sql.NullInt32

	Flagged   bool
	Duplicate bool
}

func (s Story) AgeString() string {
	return humanize.Time(time.Unix(int64(s.SubmissionTime), 0))
}

func (s Story) QualityString() string {
	return fmt.Sprintf("%.2f", s.Quality)
}
