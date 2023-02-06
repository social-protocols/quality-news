package main

type DefaultPageHeaderData struct{}

func (d DefaultPageHeaderData) IsQualityPage() bool {
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

func (d DefaultPageHeaderData) IsAboutPage() bool {
	return false
}

func (d DefaultPageHeaderData) IsDeltaPage() bool {
	return false
}
