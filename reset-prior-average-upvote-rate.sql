
with parameters as (
  select
    -- 2.2956 as priorWeight
    -- 4.0 as priorWeight
    1.7 as priorWeight
    , 0.003462767 as fatigueFactor
    -- , 1.036 as priorAverage
    -- , 1.036 as priorAverage
    -- , .99 as priorAverage
    -- , 1.0 as priorAverage
), entryRates as (
  select
    userID
    , storyID
    , entryTime
    , entryUpvoteRate
    , max(cumulativeUpvotes) cumulativeUpvotes
    , max(cumulativeExpectedUpvotes) cumulativeExpectedUpvotes
    , (cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight) newEntryUpvoteRate
    -- , (cumulativeUpvotes + priorWeight*1.174)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight) newEntryUpvoteRate
    -- , (cumulativeUpvotes + priorWeight*1.145)/(cumulativeExpectedUpvotes + priorWeight) as newEntryUpvoteRate


  from 
    votes
    join dataset 
    on dataset.id = storyID
    join parameters
  where
    dataset.sampleTime
    and sampleTime <= entryTime
    -- and votes.userID != 0
  group by userID, storyID, entryTime
)
-- select * from entryRates where userID = 0 and storyID = 36805231 limit 10;

update votes as u
set entryUpvotes = entryRates.cumulativeUpvotes
  , entryExpectedUpvotes = entryRates.cumulativeExpectedUpvotes
  , entryUpvoteRate = entryRates.newEntryUpvoteRate
from
entryRates
where entryRates.userID = u.userID
and entryRates.storyID = u.storyID ;

