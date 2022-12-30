-- this query updates cumulativeUpvotes and cumulativeExpectedUpvotes
-- accounting for possible gaps in the data (stories in the latest crawl but not the previous crawl).
-- We only want cumulativeUpvotes or cumulativeExpectedUpvotes to increase if we have two consecutive data
-- points (one minute apart).

with latest as (
	select * from dataset where sampleTime = (select max(sampleTime) from dataset)
)
update dataset as d
set
	cumulativeUpvotes = case 
		when not gapInData then previousCrawl.cumulativeUpvotes + latest.score - previousCrawl.score 
		else previousCrawl.cumulativeUpvotes
	end 
	, cumulativeExpectedUpvotes = case 
		when not gapInData then latest.cumulativeExpectedUpvotes 
		else previousCrawl.cumulativeExpectedUpvotes
	end 
from latest left join previousCrawl using (id)
where
	d.id = latest.id 
	and d.sampleTime = (select max(sampleTime) from dataset)
