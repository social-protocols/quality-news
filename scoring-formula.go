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
	var score float64
	switch formula {
	case "PTS":
		score = PeerTruthSerum(p, m) * 100
	case "InformationGain":
		score = InformationGain(p, m) * 100
	case "InformationGain2":
		score = InformationGain2(p, m) * 100
	case "InformationGain3":
		score = InformationGain3(p, m) * 100
	case "InformationGain4":
		score = InformationGain4(p, m) * 100
	case "InformationGain5":
		score = InformationGain5(p, m) * 100
	case "InformationGain6":
		score = InformationGain6(p, m) * 100
	case "InformationGain7":
		score = InformationGain7(p, m) * 100
	case "InformationGain8":
		score = InformationGain8(p, m) * 100
	case "LogPTS":
		score = LogPeerTruthSerum(p, m) * 100
	case "":
		score = LogPeerTruthSerum(p, m) * 100
	default:
		score = 0

	}

	if math.IsNaN(score) {
		fmt.Printf("Got NaN from scoring formula %s. Position: %#v. Modal Params: %#v\n", formula, p, m)
	}
	return score
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
	// Information gain logic doesn't work for downvotes
	if p.Direction == -1 {
		return 0
	}

	postEntryPrice := m.upvoteRate(p.EntryUpvotes+int(p.Direction), p.EntryExpectedUpvotes)

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


	return (postVoteUpvoteRate*ln(postEntryPrice/buyPrice(p)) + (buyPrice(p) - postEntryPrice)) / ln(2)
}

func InformationGain2(p Position, m ModelParams) float64 {
	// Information gain logic doesn't work for downvotes
	if p.Direction == -1 {
		return 0
	}

	postEntryUpvoteRate := m.upvoteRate(p.EntryUpvotes+int(p.Direction), p.EntryExpectedUpvotes)
	finalUpvoteRate := m.upvoteRate(p.CurrentUpvotes+int(p.Direction), p.CurrentExpectedUpvotes)
	if p.Exited() {
		finalUpvoteRate = m.upvoteRate(int(p.ExitUpvotes.Int64)+int(p.Direction), p.ExitExpectedUpvotes.Float64)
	}


	score := (finalUpvoteRate*ln(postEntryUpvoteRate/buyPrice(p)) + (buyPrice(p) - postEntryUpvoteRate)) / ln(2)

	if (score > 0 && finalUpvoteRate < p.EntryUpvoteRate) || (score < 0 && finalUpvoteRate > p.EntryUpvoteRate) {
		fmt.Println("Story", p.Title)
		fmt.Println("Entry", p.EntryUpvotes, p.EntryExpectedUpvotes, p.EntryUpvoteRate, m.upvoteRate(p.EntryUpvotes, p.EntryExpectedUpvotes))
		fmt.Println("Current", p.CurrentUpvotes, p.CurrentExpectedUpvotes, p.CurrentUpvoteRate)
		fmt.Println("Prices", p.EntryUpvoteRate, buyPrice(p), postEntryUpvoteRate, finalUpvoteRate)
		fmt.Println("Log PTS", p.CurrentUpvoteRate*lg(p.CurrentUpvoteRate/p.EntryUpvoteRate))
		fmt.Println("Component 1", finalUpvoteRate*lg(postEntryUpvoteRate/buyPrice(p)))
		fmt.Println("Component 2", (buyPrice(p)-postEntryUpvoteRate)/ln(2))
		// fmt.Println(fmt.Sprintf("Position %#v %f %f", p, postEntryPrice, finalUpvoteRate))
		// fmt.Println(p.EntryTime, p.EntryUpvoteRate, p.CurrentUpvoteRate, p.ExitUpvoteRate.Float64, finalUpvoteRate, postEntryPrice, buyPrice(p))
	}

	return score
}

func InformationGain3(p Position, m ModelParams) float64 {
	// Information gain logic doesn't work for downvotes
	if p.Direction == -1 {
		return 0
	}

	finalUpvoteRate := m.upvoteRate(p.CurrentUpvotes, p.CurrentExpectedUpvotes)
	if p.Exited() {
		finalUpvoteRate = m.upvoteRate(int(p.ExitUpvotes.Int64), p.ExitExpectedUpvotes.Float64)
	}

	score := (finalUpvoteRate*ln(finalUpvoteRate/buyPrice(p)) + (buyPrice(p) - finalUpvoteRate)) / ln(2)

	return score
}

// This is like InformationGain1, but we Bayesian average the postVoteUpvoteRate
func InformationGain4(p Position, m ModelParams) float64 {
	// Information gain logic doesn't work for downvotes
	if p.Direction == -1 {
		return 0
	}

	postEntryPrice := m.upvoteRate(p.EntryUpvotes+int(p.Direction), p.EntryExpectedUpvotes)

	finalUpvotes := p.CurrentUpvotes
	finalExpectedUpvotes := p.CurrentExpectedUpvotes

	if p.Exited() {
		finalUpvotes = int(p.ExitUpvotes.Int64)
		finalExpectedUpvotes = p.ExitExpectedUpvotes.Float64
	}

	if finalExpectedUpvotes == p.EntryExpectedUpvotes {
		return 0
	}

	postVoteUpvoteRate := float64(finalUpvotes-p.EntryUpvotes+4) / (finalExpectedUpvotes - p.EntryExpectedUpvotes + 4)

	return (postVoteUpvoteRate*ln(postEntryPrice/buyPrice(p)) + (buyPrice(p) - postEntryPrice)) / ln(2)
}

// This is like InformationGain4, but make downvotes work by incrementing denominator instead of
// decrementing numerator.
func InformationGain5(p Position, m ModelParams) float64 {
	// Information gain logic doesn't work for downvotes
	if p.Direction == -1 {
		return 0
	}

	postEntryPrice := m.upvoteRate(p.EntryUpvotes+1, p.EntryExpectedUpvotes)
	if p.Direction == -1 {
		postEntryPrice = m.upvoteRate(p.EntryUpvotes, p.EntryExpectedUpvotes+1)
	}

	finalUpvotes := p.CurrentUpvotes
	finalExpectedUpvotes := p.CurrentExpectedUpvotes

	if p.Exited() {
		finalUpvotes = int(p.ExitUpvotes.Int64)
		finalExpectedUpvotes = p.ExitExpectedUpvotes.Float64
	}

	if finalExpectedUpvotes == p.EntryExpectedUpvotes {
		return 0
	}

	postVoteUpvoteRate := float64(finalUpvotes-p.EntryUpvotes+4) / (finalExpectedUpvotes - p.EntryExpectedUpvotes + 4)

	return (postVoteUpvoteRate*ln(postEntryPrice/buyPrice(p)) + (buyPrice(p) - postEntryPrice)) / ln(2)
}

// This is like InformationGain1, but make downvotes work by incrementing denominator instead of
// decrementing numerator.
func InformationGain6(p Position, m ModelParams) float64 {
	// Information gain logic doesn't work for downvotes
	if p.Direction == -1 {
		return 0
	}

	postEntryPrice := m.upvoteRate(p.EntryUpvotes+1, p.EntryExpectedUpvotes)
	if p.Direction == -1 {
		postEntryPrice = m.upvoteRate(p.EntryUpvotes, p.EntryExpectedUpvotes+1)
	}

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

	return (postVoteUpvoteRate*ln(postEntryPrice/buyPrice(p)) + (buyPrice(p) - postEntryPrice)) / ln(2)
}

// This is like InformationGain5, but use the priorWeight parameter instead of random parameter 4
func InformationGain7(p Position, m ModelParams) float64 {
	// Information gain logic doesn't work for downvotes
	if p.Direction == -1 {
		return 0
	}


	postEntryPrice := m.upvoteRate(p.EntryUpvotes+1, p.EntryExpectedUpvotes)
	if p.Direction == -1 {
		postEntryPrice = m.upvoteRate(p.EntryUpvotes, p.EntryExpectedUpvotes+1)
	}

	finalUpvotes := p.CurrentUpvotes
	finalExpectedUpvotes := p.CurrentExpectedUpvotes

	if p.Exited() {
		finalUpvotes = int(p.ExitUpvotes.Int64)
		finalExpectedUpvotes = p.ExitExpectedUpvotes.Float64
	}

	if finalExpectedUpvotes == p.EntryExpectedUpvotes {
		return 0
	}

	postVoteUpvoteRate := ( float64(finalUpvotes-p.EntryUpvotes)+ m.PriorWeight) / (finalExpectedUpvotes - p.EntryExpectedUpvotes + m.PriorWeight)

	return (postVoteUpvoteRate*ln(postEntryPrice/buyPrice(p)) + (buyPrice(p) - postEntryPrice)) / ln(2)
}


// Like InformationGain1, but takes weighted average of score and 0 (weighted by post-entry expected upvotes) 
func InformationGain8(p Position, m ModelParams) float64 {
	// Information gain logic doesn't work for downvotes
	if p.Direction == -1 {
		return 0
	}

	postEntryPrice := m.upvoteRate(p.EntryUpvotes+int(p.Direction), p.EntryExpectedUpvotes)

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


	gain := (postVoteUpvoteRate*ln(postEntryPrice/buyPrice(p)) + (buyPrice(p) - postEntryPrice)) / ln(2)

	return ( gain + 0 ) / ( finalExpectedUpvotes - p.EntryExpectedUpvotes + m.PriorWeight )

}

func LogPeerTruthSerum(p Position, m ModelParams) float64 {
	postEntryPrice := m.upvoteRate(p.CumulativeUpvotes+1, p.CumulativeExpectedUpvotes)

	if p.ID == 36805231 {
		fmt.Println("Prices", p.EntryUpvoteRate, postEntryPrice, p.CurrentUpvoteRate, buyPrice(p), postEntryPrice, sellPrice(p), postEntryPrice/buyPrice(p), sellPrice(p)/buyPrice(p))
	}

	if p.ID == 36731752 {
		fmt.Println("Entry", p.Title, p.EntryUpvotes, p.EntryExpectedUpvotes, p.EntryUpvoteRate, m.upvoteRate(p.EntryUpvotes, p.EntryExpectedUpvotes))
		fmt.Println("Prices", sellPrice(p), buyPrice(p), sellPrice(p)/buyPrice(p), lg(sellPrice(p)/buyPrice(p)))
	}

	return lg(sellPrice(p) / buyPrice(p))
}

func PeerTruthSerum(p Position, m ModelParams) float64 {
	if p.ID == 36805231 {
		fmt.Println("Prices", p.EntryUpvoteRate, p.CurrentUpvoteRate, buyPrice(p), sellPrice(p), sellPrice(p)/buyPrice(p))
	}

	return sellPrice(p)/buyPrice(p) - 1
}
