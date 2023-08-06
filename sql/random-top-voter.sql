with randomDatapoints as (
  select 
    id, sampleTime , cumulativeUpvotes, cumulativeExpectedUpvotes
    -- , row_number() over () as 
    , row_number() over () as i
    , count() over () as nIDs
  from dataset 
  join stories using (id)
  where
  timestamp > ( select min(sampleTime) from dataset ) -- only stories submitted since we started crawling
  and sampleTime > ( select max(sampleTime) from dataset ) - 24 * 60 * 60
  and topRank is not null 
), 
 limits as (
  select abs(random()) % ( nIds / 100 ) as n
  from randomDatapoints
  where i = 1
)
, storiesToUpvote as (
  select id as storyID
    , min(sampleTime) as minSampleTime
    , min(cumulativeUpvotes) as minUpvotes
    , min(cumulativeExpectedUpvotes) as minExpectedUpvotes
  from randomDatapoints join limits
  -- sampleTime % nIDs = n
  where
   ( i ) % (nIDs / 100) = n
  group by id
  order by sampleTime
)
, positions as (
  select 
    ? as userID
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
  order by entryTime desc;