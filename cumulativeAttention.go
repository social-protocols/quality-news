package main

import (
	"math"
)

const nPages = 3     // page 1 (rank 1-30), page 2, ...
const nPageTypes = 5 // new, top, etc
type cs struct {
	rankCoefficient float64
	pageCoefficient float64
}
type csMatrix [nPageTypes][nPages]cs

var coefficients = csMatrix{
	[nPages]cs{{1, 2}, {1, 2}, {1, 2}},
	[nPages]cs{{1, 2}, {1, 2}, {1, 2}},
	[nPages]cs{{1, 2}, {1, 2}, {1, 2}},
	[nPages]cs{{1, 2}, {1, 2}, {1, 2}},
	[nPages]cs{{1, 2}, {1, 2}, {1, 2}},
}

func accumulateAttention(ndb newsDatabase, logger leveledLogger, pageType int, storyID int, rank int, sampleTime int64, upvotes int, submissionTime int64) {
	page := (rank - 1) / 30

	rankCoefficient := coefficients[pageType][page].rankCoefficient
	pageCoefficient := coefficients[pageType][page].pageCoefficient
	deltaAttention := math.Exp(math.Log(float64(page))*pageCoefficient + math.Log(float64(rank))*rankCoefficient)

	err := ndb.upsertAttention(storyID, upvotes, submissionTime, deltaAttention, sampleTime)
	if err != nil {
		logger.Err(err)
	}
}
