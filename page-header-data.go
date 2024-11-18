package main

import "database/sql"

type DefaultPageHeaderData struct{ UserID sql.NullInt64 }

func (d DefaultPageHeaderData) IsFairPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsUpvoteratePage() bool {
	return false
}

func (d DefaultPageHeaderData) IsHNTopPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsNewPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsBestPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsAskPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsShowPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsRawPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsPenaltiesPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsBoostsPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsResubmissionsPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsAboutPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsAlgorithmsPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsScorePage() bool {
	return false
}

func (d DefaultPageHeaderData) IsAlternativeFrontPage() bool {
	return d.IsHNTopPage() || d.IsRawPage() || d.IsPenaltiesPage() || d.IsBoostsPage() || d.IsResubmissionsPage() || d.IsFairPage() || d.IsUpvoteratePage()
}
