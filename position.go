package main

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	humanize "github.com/dustin/go-humanize"
)

type Position struct {
	UserID               int
	StoryID              int
	PositionID           int
	Direction            int8
	EntryTime            int64
	EntryUpvotes         float64
	EntryExpectedUpvotes float64
	EntryUpvoteRate      float64
	PostEntryUpvoteRate  float64
	ExitTime             sql.NullInt64
	ExitUpvoteRate       sql.NullFloat64
	CurrentUpvoteRate    float64
	SubsequentUpvoteRate float64
	Story
	RunningScore float64
	Label        string
}


func (p Position) VoteTypeString() string {
	switch p.Direction {
	case 1:
		return "upvoted"
	case -1:
		return "downvoted"
	default:
		panic("Invalid direction value")
	}
}

func (p Position) EntryTimeString() string {
	return humanize.Time(time.Unix(p.EntryTime, 0))
}

func (p Position) EntryUpvoteRateString() string {
	return fmt.Sprintf("%.2f", p.EntryUpvoteRate)
}

func (p Position) Exited() bool {
	return p.ExitTime.Valid
}

func (p Position) ExitTimeString() string {
	// return time.Unix(int64(s.MaxSampleTime), 0).UTC().Format("2006-01-02T15:04")
	if !p.ExitTime.Valid {
		return ""
	}
	return humanize.Time(time.Unix(p.ExitTime.Int64, 0))
}

func (p Position) ExitUpvoteRateString() string {
	if !p.ExitUpvoteRate.Valid {
		return ""
	}
	return fmt.Sprintf("%.2f", p.ExitUpvoteRate.Float64)
}

func (p Position) CurrentUpvoteRateString() string {
	return fmt.Sprintf("%.2f", p.CurrentUpvoteRate)
}

func (p Position) sellPrice() float64 {
	// upvoteRateAdjustment := 0.7203397586203779

	if p.Direction == -1 {
		return p.EntryUpvoteRate
	} else if p.Exited() {
		return p.ExitUpvoteRate.Float64
	}
	return p.CurrentUpvoteRate
}

func (p Position) buyPrice() float64 {
	if p.Direction == 1 {
		return p.EntryUpvoteRate
	} else if p.Exited() {
		return p.ExitUpvoteRate.Float64
	}
	return p.CurrentUpvoteRate
}

func (p Position) Gain() float64 {
	// return p.InformationDistance() * 100
	return p.LogPeerTruthSerum()*100
	// return p.PeerTruthSerum()*100
}

func ln(v float64) float64 {
	return math.Log(v)
}

func lg(v float64) float64 {
	return math.Log(v) / math.Log(2)
}

func (p Position) InformationDistance() float64 {
	// return p.sellPrice()/p.buyPrice()*100 - 100

	// postEntryPrice := (p.EntryUpvotes+1) / (p.EntryExpectedUpvotes)

	postEntryPrice := p.PostEntryUpvoteRate

	// p.sellPrice() * ( log()

	if p.ID == 36805231 {
		fmt.Println("Prices", p.EntryUpvoteRate, p.PostEntryUpvoteRate, p.CurrentUpvoteRate, p.buyPrice(), postEntryPrice, p.sellPrice(), postEntryPrice/p.buyPrice(), p.sellPrice()/p.buyPrice())
	}

	// return ( math.Log(postEntryPrice/p.buyPrice()) - math.Log(p.sellPrice()/p.buyPrice()))  *100

	return (p.sellPrice()*ln(postEntryPrice/p.buyPrice()) + (p.buyPrice() - postEntryPrice)) / ln(2)
}

func (p Position) LogPeerTruthSerum() float64 {
	// return p.sellPrice()/p.buyPrice()*100 - 100

	// postEntryPrice := (p.EntryUpvotes+1) / (p.EntryExpectedUpvotes)

	postEntryPrice := p.PostEntryUpvoteRate

	// p.sellPrice() * ( log()

	if p.ID == 36805231 {
		fmt.Println("Prices", p.EntryUpvoteRate, p.PostEntryUpvoteRate, p.CurrentUpvoteRate, p.buyPrice(), postEntryPrice, p.sellPrice(), postEntryPrice/p.buyPrice(), p.sellPrice()/p.buyPrice())
	}

	// return ( math.Log(postEntryPrice/p.buyPrice()) - math.Log(p.sellPrice()/p.buyPrice()))  *100

	return lg(p.sellPrice() / p.buyPrice())
}

func (p Position) PeerTruthSerum() float64 {
	// return p.sellPrice()/p.buyPrice()*100 - 100

	// postEntryPrice := (p.EntryUpvotes+1) / (p.EntryExpectedUpvotes)

	postEntryPrice := p.PostEntryUpvoteRate

	// p.sellPrice() * ( log()

	if p.ID == 36805231 {
		fmt.Println("Prices", p.EntryUpvoteRate, p.PostEntryUpvoteRate, p.CurrentUpvoteRate, p.buyPrice(), postEntryPrice, p.sellPrice(), postEntryPrice/p.buyPrice(), p.sellPrice()/p.buyPrice())
	}

	// return ( math.Log(postEntryPrice/p.buyPrice()) - math.Log(p.sellPrice()/p.buyPrice()))  *100

	return p.sellPrice()/p.buyPrice() - 1
}

func (p Position) GainString() string {
	gain := p.Gain()

	if math.Abs(gain) < .01 {
		return "-"
	}

	if gain > 0 {
		return fmt.Sprintf("+%.2f", gain)
	}

	return fmt.Sprintf("%.2f", gain)
}

func (p Position) IsGain() bool {
	isGain := p.sellPrice() > p.buyPrice()

	return isGain
}
