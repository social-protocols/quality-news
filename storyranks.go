type StoryRanks struct {
	TopRank int
	NewRank int
	BestRank int
	AskRank int
	ShowRank int
}

type Datapoint {
	StoryRanks
	ID int
	SubmissionTime int
	CrawlTime int
}


type StoryRanks [5]int

storyRanksMap := make(map[int]StoryRanks)

pageTypes := map[int]string{
	0: "top",
	1: "new",
}


for pageType, pageTypeString := range pageTypes {
	ids := c.GetStories(pageTypeString)

	for i, id := range ids
		var storyRanks StoryRanks

		if storyRank, ok = storyRanksMap[id]; !ok {
			storyRanks = StoryRanks{}
		}

		storyRanks[pageType] = i + 1

		storyRanksMap[id] = storyRanks
}

for storyID, ranks := range storyRanksMap {
	// write to databse
	

}
