delete from votes where userID = 0;

with s as (
	select 
		id storyID
		, min(sampleTime) as minSampleTime
	from dataset
	group by storyID
)	
insert into votes select 0 as userID, storyID, 1 as direction, minSampleTime as entryTime, 0 as entryUpvotes, 0 as entryExpectedUpvotes, 1.0 as entryUpvoteRate from s where storyID >= (select min(storyID) from votes where userID = 1) and storyID <= (select max(storyID) from votes where userID = 1) order by storyID asc;
