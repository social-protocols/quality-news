with limits as (
  select
    count(*) / 1000 as n
    , abs(random()) % 10 as m
  from dataset
)
, randomFrontpageSample as (
  select id, sampleTime, cumulativeUpvotes, cumulativeExpectedUpvotes
  from dataset 
  join stories using (id)
  join limits
  where timestamp > ( select min(sampleTime) from dataset ) -- only stories submitted since we started crawling
  and newRank is not null 
  and not job
  and ( ( dataset.rowid - (select min(rowid) from dataset) )  %  n ) = m
)
, storiesToUpvote as (
  select id as storyID
    , min(sampleTime) as minSampleTime
    , min(cumulativeUpvotes) as minUpvotes
    , min(cumulativeExpectedUpvotes) as minExpectedUpvotes
  from randomFrontpageSample
  group by id
  order by sampleTime
)
, positions as (
  select 
    0 as userID
    , storiesToUpvote.storyID
    , 1 as direction
    , minSampleTime as entryTime
    , minUpvotes as entryUpvotes
    , minExpectedUPvotes as entryExpectedUpvotes
    , row_number() over () as positionID
  from storiesToUpvote
  -- left join votes existingVotes using (storyID)
  -- where existingVotes.storyID is null
) select
  userID
  , storyID
  , positionID
  , direction
  , entryTime
  , entryUpvotes
  , entryExpectedUpvotes
  , null as exitTime
  , null as exitUpvotes
  , null as exitExpectedUpvotes
  , cumulativeUpvotes
  , cumulativeExpectedUpvotes
  , title
  , url
  , by
  , unixepoch() - sampleTime + coalesce(ageApprox, sampleTime - submissionTime) ageApprox
  , score
  , descendants as comments
  from positions 
  join dataset on 
    positions.storyID = id
  join stories using (id)
  group by positionID
  having max(dataset.sampleTime)
  order by entryTime desc
;
