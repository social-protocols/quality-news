
delete from votes where userID = 0;

with randomFrontpageStories as (
  select id, sampleTime , cumulativeUpvotes, cumulativeExpectedUpvotes
  from dataset 
  join stories using (id)
  where timestamp > (select min(sampleTime) from dataset) -- only stories submitted since we started crawling
  and topRank is not null 
  and not job
  and dataset.rowid % ( (select count(*) from dataset)/1000 ) = ( abs(random()) % 100 )
), s as (
  select id as storyID
    , min(sampleTime) as minSampleTime
    , min(cumulativeUpvotes) as minUpvotes
    , min(cumulativeExpectedUpvotes) as minExpectedUpvotes
  from randomFrontpageStories
  group by id
  order by sampleTime
)
insert into votes 
select 
  0 as userID
  , s.storyID
  , 1 as direction
  , minSampleTime as entryTime
  , minUpvotes as entryUpvotes
  , minExpectedUPvotes as entryExpectedUpvotes
from s 
left join votes existingVotes using (storyID)
where existingVotes.storyID is null;
