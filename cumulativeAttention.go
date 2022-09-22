package main

import (
	"github.com/pkg/errors"
	"math"
)

const nPages = 3     // page 1 (rank 1-30), page 2, ...
const nPageTypes = 5 // new, top, etc

type pageCoefficients = struct {
	pageTypeCoefficient float64
	pageCoefficient     float64
	rankCoefficients    [nPages]float64
}

// These coefficients are the output of regression-pagetype-page-and-rank.R
// from the hacker-news-data repository. The rank coefficients for the
// ask and best pages looked pretty questionable, due to a sparsity of data,
// so I manually estimated some values
var coefficients = [nPageTypes]pageCoefficients{
	{0.13963, -3.54835, [nPages]float64{-0.64128, -0.36647, -0.19429}},
	{-3.05791, -2.45658, [nPages]float64{-0.64681, -0.29721, -0.11946}},
	{-4.25609, -1.84011, [nPages]float64{-0.44322, -0.16904, -0.08}},
	{-2.46316, -5.67281, [nPages]float64{-0.38478, -0.14, -0.06}},
	{-5.40623, -4.08824, [nPages]float64{-4, -0.2, -0.05}},
}

func accumulateAttention(ndb newsDatabase, logger leveledLogger, pageType int, storyID int, oneBasedRank int, sampleTime int64, deltaUpvotes int, totalUpvotes int, totalComments int, submissionTime int64) {
	zeroBasedPage := (oneBasedRank - 1) / 30

	cs := coefficients[pageType]

	logger.Debug("Calculating cumulative attention", "oneBasedPage", zeroBasedPage+1, "oneBasedRank", oneBasedRank, "cs", cs, "term2", cs.pageCoefficient*math.Log(float64(zeroBasedPage+1)), "term3", cs.rankCoefficients[zeroBasedPage]*math.Log(float64(oneBasedRank)))

	deltaAttention := math.Exp(cs.pageTypeCoefficient - cs.pageCoefficient*math.Log(float64(zeroBasedPage+1)) - cs.rankCoefficients[zeroBasedPage]*math.Log(float64(oneBasedRank)))

	err := ndb.upsertAttention(storyID, deltaUpvotes, totalUpvotes, totalComments, submissionTime, deltaAttention, sampleTime)
	if err != nil {
		logger.Err(errors.Wrap(err, "upsertAttention"))
	}
}
