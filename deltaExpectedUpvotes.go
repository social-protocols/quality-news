package main

import (
	"math"
)

const (
	nPages     = 3 // page 1 (rank 1-30), page 2, ...
	nPageTypes = 5 // new, top, etc
)

type pageCoefficients = struct {
	pageTypeCoefficient float64
	pageCoefficient     float64
	rankCoefficient     float64
}

// These coefficients are the output of bayesian-quality-pagetype-rank.R
// from the hacker-news-data repository.
var (
	coefficients = [nPageTypes]pageCoefficients{
		{-2.733096, -3.492384, -0.5636350},
		{-5.806347, -2.680377, -0.3879157},
		{-7.365239, -1.141086, -0.2927700},
		{-5.743499, -4.986510, -1.0510611},
		{-7.237460, -4.884862, -0.8878165},
	}
)

func expectedUpvoteShare(pageType, oneBasedRank int) float64 {
	zeroBasedPage := (oneBasedRank - 1) / 30
	oneBasedRankOnPage := ((oneBasedRank - 1) % 30) + 1

	cs := coefficients[pageType]

	logExpectedUpvoteShare := cs.pageTypeCoefficient +
		cs.pageCoefficient*math.Log(float64(zeroBasedPage+1)) +
		cs.rankCoefficient*math.Log(float64(oneBasedRankOnPage))/float64(zeroBasedPage+1)

	return math.Exp(logExpectedUpvoteShare)
}
