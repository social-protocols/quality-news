package main

import (
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
	"time"
)

var randSrc = rand.NewSource(uint64(time.Now().UnixNano()))

func sampleFromGammaDistribution(alpha, beta float64) float64 {
	dist := distuv.Gamma{
		Alpha: alpha,
		Beta:  beta,
		Src:   randSrc,
	}
	return dist.Rand()
}
