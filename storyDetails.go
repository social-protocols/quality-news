package main

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/weppos/publicsuffix-go/publicsuffix"

	humanize "github.com/dustin/go-humanize"
)

type Story struct {
	ID                     int
	By                     string
	Title                  string
	URL                    string
	SubmissionTime         int64
	OriginalSubmissionTime int64
	AgeApprox              int64
	Score                  int
	Comments               int
	Quality                float64
	Penalty                float64
	TopRank                sql.NullInt32
	QNRank                 sql.NullInt32
}

func (s Story) AgeString() string {
	return humanize.Time(time.Unix(int64(time.Now().Unix()-s.AgeApprox), 0))
}

func (s Story) OriginalAgeString() string {
	return humanize.Time(time.Unix(s.OriginalSubmissionTime, 0))
}

func (s Story) IsResubmitted() bool {
	return s.SubmissionTime != s.OriginalSubmissionTime
}

func (s Story) QualityString() string {
	return fmt.Sprintf("%.2f", s.Quality)
}

func (s Story) Domain() string {
	u, err := url.Parse(s.URL)
	if err != nil {
		return ""
	}

	domain, err := publicsuffix.Domain(u.Host)
	if err != nil {
		return ""
	}

	// some domains are treated specially:

	// twitter.com/x
	// github.com/x
	// x.substack.com
	// x.notion.site
	// x.dreamhosters.com

	if u.Host == "news.ycombinator.com" {
		return ""
	}

	if domain == "twitter.com" || domain == "github.com" {
		// keep first part of path
		return domain + "/" + strings.Split(u.Path, "/")[1]
	}

	if domain == "substack.com" || domain == "notion.site" || domain == "dreamhosters.com" {
		// keep subdomain
		return strings.Split(u.Host, ".")[0] + "." + domain
	}

	return domain
}

func (s Story) ISOTimestamp() string {
	return time.Unix(s.SubmissionTime, 0).UTC().Format("2006-01-02T15:04:05")
}

func (s Story) OriginalISOTimestamp() string {
	return time.Unix(s.OriginalSubmissionTime, 0).UTC().Format("2006-01-02T15:04:05")
}
