package main

import (
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
	TopRank        int32
	QNRank         int32
}

func (s Story) AgeString() string {
	return humanize.Time(time.Unix(int64(s.SubmissionTime), 0))
}

func (s Story) QualityString() string {
	return fmt.Sprintf("%.2f", s.Quality)
}

func (s Story) HNRankString() string {

	// if s.TopRank == -1 { return "" }
	//⨂

	if s.TopRank == 0 {
		return ""
	}

	return fmt.Sprintf("%d", s.TopRank)
}

func (s Story) QNRankString() string {

	// if s.QNRank == -1 { return "" }

	if s.QNRank == 0 {
		return ""
	}

	return fmt.Sprintf("%d", s.QNRank)
}
