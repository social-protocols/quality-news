
delete from votes where userID = 1;


with limits as (
  select
    count(*) / 1000 as n
    , abs(random()) % 10 as m
  from dataset
)
, randomFrontpageStories as (
  select id, sampleTime , cumulativeUpvotes, cumulativeExpectedUpvotes
  from dataset 
  join stories using (id)
  join limits
  where timestamp > ( select min(sampleTime) from dataset ) -- only stories submitted since we started crawling
  and newRank is not null 
  and not job
  and ( ( dataset.rowid - (select min(rowid) from dataset) )  %  n ) = m
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
  1 as userID
  , s.storyID
  , 1 as direction
  , minSampleTime as entryTime
  , minUpvotes as entryUpvotes
  , minExpectedUPvotes as entryExpectedUpvotes
from s 
left join votes existingVotes using (storyID)
where existingVotes.storyID is null;
