package main

import (
	"fmt"
	"math"
)

func sellPrice(p Position) float64 {
	// upvoteRateAdjustment := 0.7203397586203779

	if p.Direction == -1 {
		return p.EntryUpvoteRate
	} else if p.Exited() {
		return p.ExitUpvoteRate.Float64
	}
	return p.CurrentUpvoteRate
}

func buyPrice(p Position) float64 {
	if p.Direction == 1 {
		return p.EntryUpvoteRate
	} else if p.Exited() {
		return p.ExitUpvoteRate.Float64
	}
	return p.CurrentUpvoteRate
}

func UserScore(p Position, m ModelParams, formula string) float64 {
	switch formula {
	case "PTS":
		return PeerTruthSerum(p, m) * 100
	case "InformationGain":
		return InformationGain(p, m) * 100
		// case "LogPTS":
	default:
		return LogPeerTruthSerum(p, m) * 100
	}
	// return p.LogPeerTruthSerum() * 100
	// return p.PeerTruthSerum()*100
}

func ln(v float64) float64 {
	return math.Log(v)
}

func lg(v float64) float64 {
	return math.Log(v) / math.Log(2)
}

func InformationGain(p Position, m ModelParams) float64 {
	// return sellPrice(p)/buyPrice(p)*100 - 100

	// postEntryPrice := (p.EntryUpvotes+1) / (p.EntryExpectedUpvotes)

	postEntryPrice := m.upvoteRate(p.EntryUpvotes+int(p.Direction), p.EntryExpectedUpvotes)
	// finalPrice := m.upvoteRate(p.CurrentUpvotes+int(p.Direction), p.CurrentExpectedUpvotes)
	// if p.Exited() {
	// 	finalPrice = m.upvoteRate(int(p.ExitUpvotes.Int64), p.ExitExpectedUpvotes.Float64)
	// }

	finalUpvotes := p.CurrentUpvotes
	finalExpectedUpvotes := p.CurrentExpectedUpvotes

	if p.Exited() {
		finalUpvotes = int(p.ExitUpvotes.Int64)
		finalExpectedUpvotes = p.ExitExpectedUpvotes.Float64
	}

	if finalExpectedUpvotes == p.EntryExpectedUpvotes {
		return 0
	}

	postVoteUpvoteRate := float64(finalUpvotes-p.EntryUpvotes) / (finalExpectedUpvotes - p.EntryExpectedUpvotes)
	// postVoteUpvoteRate := m.upvoteRate(finalUpvotes, finalExpectedUpvotes)

	// This hack doesn't make sense.
	if postEntryPrice < 0 {
		return 0
	}

	if p.ID == 36731752 {
		fmt.Println("Story", p.Title)
		fmt.Println("Entry", p.EntryUpvotes, p.EntryExpectedUpvotes, p.EntryUpvoteRate, m.upvoteRate(p.EntryUpvotes, p.EntryExpectedUpvotes))
		fmt.Println("Current", p.CurrentUpvotes, p.CurrentExpectedUpvotes, p.CurrentUpvoteRate)
		fmt.Println("Prices", p.EntryUpvoteRate, buyPrice(p), postEntryPrice, postVoteUpvoteRate)
		fmt.Println("Log PTS", p.CurrentUpvoteRate*lg(p.CurrentUpvoteRate/p.EntryUpvoteRate))
		fmt.Println("Component 1", postVoteUpvoteRate*lg(postEntryPrice/buyPrice(p)))
		fmt.Println("Component 2", (buyPrice(p)-postEntryPrice)/ln(2))
		// fmt.Println(fmt.Sprintf("Position %#v %f %f", p, postEntryPrice, finalPrice))
		// fmt.Println(p.EntryTime, p.EntryUpvoteRate, p.CurrentUpvoteRate, p.ExitUpvoteRate.Float64, finalPrice, postEntryPrice, buyPrice(p))
	}

	return (postVoteUpvoteRate*ln(postEntryPrice/buyPrice(p)) + (buyPrice(p) - postEntryPrice)) / ln(2)
}

func LogPeerTruthSerum(p Position, m ModelParams) float64 {
	// return sellPrice(p)/buyPrice(p)*100 - 100

	// postEntryPrice := (p.EntryUpvotes+1) / (p.EntryExpectedUpvotes)

	// postEntryPrice := p.PostEntryUpvoteRate
	postEntryPrice := m.upvoteRate(p.CumulativeUpvotes+1, p.CumulativeExpectedUpvotes)

	// sellPrice(p) * ( log()

	if p.ID == 36805231 {
		fmt.Println("Prices", p.EntryUpvoteRate, postEntryPrice, p.CurrentUpvoteRate, buyPrice(p), postEntryPrice, sellPrice(p), postEntryPrice/buyPrice(p), sellPrice(p)/buyPrice(p))
	}

	if p.ID == 36731752 {
		fmt.Println("Entry", p.Title, p.EntryUpvotes, p.EntryExpectedUpvotes, p.EntryUpvoteRate, m.upvoteRate(p.EntryUpvotes, p.EntryExpectedUpvotes))
		fmt.Println("Prices", sellPrice(p), buyPrice(p), sellPrice(p)/buyPrice(p), lg(sellPrice(p)/buyPrice(p)))
	}

	// return ( math.Log(postEntryPrice/buyPrice(p)) - math.Log(sellPrice(p)/buyPrice(p)))  *100

	return lg(sellPrice(p) / buyPrice(p))
}

func PeerTruthSerum(p Position, m ModelParams) float64 {
	// return sellPrice(p)/buyPrice(p)*100 - 100

	// postEntryPrice := (p.EntryUpvotes+1) / (p.EntryExpectedUpvotes)

	// sellPrice(p) * ( log()

	if p.ID == 36805231 {
		fmt.Println("Prices", p.EntryUpvoteRate, p.CurrentUpvoteRate, buyPrice(p), sellPrice(p), sellPrice(p)/buyPrice(p))
	}

	// return ( math.Log(postEntryPrice/buyPrice(p)) - math.Log(sellPrice(p)/buyPrice(p)))  *100

	return sellPrice(p)/buyPrice(p) - 1
}
