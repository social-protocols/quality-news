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
	ID                        int
	By                        string
	Title                     string
	URL                       string
	SubmissionTime            int64
	OriginalSubmissionTime    int64
	AgeApprox                 int64
	Score                     int
	Comments                  int
	CumulativeUpvotes         int
	CumulativeExpectedUpvotes float64
	UpvoteRate                float64
	TopRank                   sql.NullInt32
	QNRank                    sql.NullInt32
	RawRank                   sql.NullInt32
	Job                       bool
	Flagged                   bool
	Dupe                      bool
	Archived                  bool
}

// PageTemplateData contains the common template data for all pages
type PageTemplateData struct {
	Ranking string
	UserID  sql.NullInt64
}

// StoryTemplateData combines a Story with page context for use in templates
type StoryTemplateData struct {
	Story // embed Story instead of having it as a named field
	PageTemplateData
}

// Page-specific methods for ranking-based pages
func (p PageTemplateData) IsHNTopPage() bool {
	return p.Ranking == "hntop"
}

func (p PageTemplateData) IsFairPage() bool {
	return p.Ranking == "fair"
}

func (p PageTemplateData) IsUpvoteratePage() bool {
	return p.Ranking == "upvoterate"
}

func (p PageTemplateData) IsBestUpvoteratePage() bool {
	return p.Ranking == "best-upvoterate"
}

func (p PageTemplateData) IsNewPage() bool {
	return p.Ranking == "new"
}

func (p PageTemplateData) IsBestPage() bool {
	return p.Ranking == "best"
}

func (p PageTemplateData) IsAskPage() bool {
	return p.Ranking == "ask"
}

func (p PageTemplateData) IsShowPage() bool {
	return p.Ranking == "show"
}

func (p PageTemplateData) IsRawPage() bool {
	return p.Ranking == "raw"
}

func (p PageTemplateData) IsPenaltiesPage() bool {
	return p.Ranking == "penalties"
}

func (p PageTemplateData) IsBoostsPage() bool {
	return p.Ranking == "boosts"
}

func (p PageTemplateData) IsResubmissionsPage() bool {
	return p.Ranking == "resubmissions"
}

// Default implementations for non-ranking based pages
func (p PageTemplateData) IsAboutPage() bool {
	return false
}

func (p PageTemplateData) IsAlgorithmsPage() bool {
	return false
}

func (p PageTemplateData) IsScorePage() bool {
	return false
}

func (p PageTemplateData) IsStatsPage() bool {
	return false
}

func (p PageTemplateData) IsAlternativeFrontPage() bool {
	return p.IsHNTopPage() || p.IsRawPage() || p.IsPenaltiesPage() || p.IsBoostsPage() || p.IsResubmissionsPage() || p.IsFairPage() || p.IsUpvoteratePage() || p.IsBestUpvoteratePage() || p.IsNewPage() || p.IsBestPage() || p.IsAskPage() || p.IsShowPage()
}

func (s Story) AgeString() string {
	// return humanize.Time(time.Unix(int64(time.Now().Unix()-s.AgeApprox), 0))
	return humanize.Time(time.Unix(int64(time.Now().Unix()-s.AgeApprox), 0))
}

func (s Story) OriginalAgeString() string {
	return humanize.Time(time.Unix(s.OriginalSubmissionTime, 0))
}

func (s Story) IsResubmitted() bool {
	return s.SubmissionTime != s.OriginalSubmissionTime
}

func (s Story) UpvoteRateString() string {
	return fmt.Sprintf("%.2f", s.UpvoteRate)
}

func (s Story) RankDiff() int32 {
	if !s.RawRank.Valid {
		return 0
	}
	rawRank := s.RawRank.Int32
	topRank := s.TopRank.Int32

	if !s.TopRank.Valid {
		topRank = 91
	}

	return rawRank - topRank
}

func abs(a int32) int32 {
	if a >= 0 {
		return a
	}
	return -a
}

func (s Story) RankDiffAbs() int32 {
	return abs(s.RankDiff())
}

func (s Story) OverRanked() bool {
	return s.RankDiff() > 0
}

func (s Story) UnderRanked() bool {
	return s.RankDiff() < 0
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
