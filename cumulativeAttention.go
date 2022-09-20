package main

import (
	"math"
)

const nPages = 3     // page 1 (rank 1-30), page 2, ...
const nPageTypes = 5 // new, top, etc

type pageCoefficients = struct {
	pageTypeCoefficient float64
	pageCoefficient     float64
	rankCoefficients    [nPages]float64
}

var coefficients = [nPageTypes]pageCoefficients{
	{2.0, 0.3, [nPages]float64{.3, .15, .6}},
	{1.0, .3, [nPages]float64{.3, .15, .6}},
	{0.5, .3, [nPages]float64{.3, .15, .6}},
	{0.4, .3, [nPages]float64{.3, .15, .6}},
	{0.3, .3, [nPages]float64{.3, .15, .6}},
}

func accumulateAttention(ndb newsDatabase, logger leveledLogger, pageType int, storyID int, oneBasedRank int, sampleTime int64, upvotes int, submissionTime int64) {
	zeroBasedPage := (oneBasedRank - 1) / 30

	cs := coefficients[pageType]

	logger.Debug("Calculating cumulative attention", "oneBasedPage", zeroBasedPage+1, "oneBasedRank", oneBasedRank, "cs", cs, "term2", cs.pageCoefficient*math.Log(float64(zeroBasedPage+1)), "term3", cs.rankCoefficients[zeroBasedPage]*math.Log(float64(oneBasedRank)))

	deltaAttention := math.Exp(cs.pageTypeCoefficient - cs.pageCoefficient*math.Log(float64(zeroBasedPage+1)) - cs.rankCoefficients[zeroBasedPage]*math.Log(float64(oneBasedRank)))

	err := ndb.upsertAttention(storyID, upvotes, submissionTime, deltaAttention, sampleTime)
	if err != nil {
		logger.Err(err)
	}
}
