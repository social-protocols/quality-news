package main

import (
	"database/sql"
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
		// {-2.733096, -3.492384, -0.5636350},
		// {-5.806347, -2.680377, -0.3879157},
		// {-7.365239, -1.141086, -0.2927700},
		// {-5.743499, -4.986510, -1.0510611},
		// {-7.237460, -4.884862, -0.8878165},
		{-2.886938, -3.316492, -0.5193376},
		{-5.856364, -2.564690, -0.3937709},
		{-7.175409, -1.280364, -0.3717084},
		{-5.316879, -5.469948, -1.2944215},
		{-6.292276, -5.912105, -1.1996512},
	}
	// fatigueFactor = 0.003462767
	// priorWeight = 1.7
	// priorWeight = 2.2956
	// priorWeight = 0.5
)

type ModelParams struct {
	FatigueFactor float64
	PriorWeight   float64
}

type OptionalModelParams struct {
	FatigueFactor sql.NullFloat64
	PriorWeight   sql.NullFloat64
}

func (p OptionalModelParams) WithDefaults() ModelParams {
	var results ModelParams

	if p.PriorWeight.Valid {
		results.PriorWeight = p.PriorWeight.Float64
	} else {
		results.PriorWeight = defaultModelParams.PriorWeight
	}

	if p.FatigueFactor.Valid {
		results.FatigueFactor = p.FatigueFactor.Float64
	} else {
		results.FatigueFactor = defaultModelParams.FatigueFactor
	}

	return results
}

var defaultModelParams = ModelParams{0.003462767, 2.2956}

func (p ModelParams) upvoteRate(upvotes int, expectedUpvotes float64) float64 {
	return (float64(upvotes) + p.PriorWeight) / float64((1-math.Exp(-p.FatigueFactor*expectedUpvotes))/p.FatigueFactor+p.PriorWeight)
}

func expectedUpvoteShare(pageType pageTypeInt, oneBasedRank int) float64 {
	zeroBasedPage := (oneBasedRank - 1) / 30
	oneBasedRankOnPage := ((oneBasedRank - 1) % 30) + 1

	cs := coefficients[pageType]

	logExpectedUpvoteShare := cs.pageTypeCoefficient +
		cs.pageCoefficient*math.Log(float64(zeroBasedPage+1)) +
		cs.rankCoefficient*math.Log(float64(oneBasedRankOnPage))/float64(zeroBasedPage+1)

	return math.Exp(logExpectedUpvoteShare)
}

var averageCrawlDelay = 10

func expectedUpvoteShareNewPage(oneBasedRank, elapsedTime int, newRankChanges []int) float64 {
	rank := oneBasedRank
	exUpvoteShare := 0.0

	for j, current := range append(newRankChanges, elapsedTime+10) {

		r := rank - j
		if r < 1 {
			break
		}

		var previous int
		var timeAtRank int

		// Calculate the value of the variable previous, which is how many
		// seconds ago this story moved out of rank r
		if j > 0 {
			previous = newRankChanges[j-1]
		} else {
			// Most stories don't appear on the new page until about 10 seconds after submission.
			// So subtract 10 seconds from the age of the story at rank 1.
			previous = averageCrawlDelay
		}

		if current > elapsedTime+averageCrawlDelay {
			current = elapsedTime + averageCrawlDelay
		}
		timeAtRank = current - previous
		if timeAtRank <= 0 {
			// Some stories might appear on the new page after less than averageCrawlDelay seconds. So by subtracting averageCrawlDelay seconds from
			// the submission time, we can end up with a negative timeAtRank. But this needs to be positive,
			// because total attentionShare must be greater than zero. So instead of subtracting averageCrawlDelay, divide by 2.
			timeAtRank = current / 2
		}

		exUpvoteShare += expectedUpvoteShare(1, r) * float64(timeAtRank) / float64(elapsedTime)
	}

	return exUpvoteShare
}
