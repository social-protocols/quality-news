/*Calculate the moving average upvote rate. The moving average window is based
  on expected upvotes, instead of time. As a result, the length of the window
  in terms of number of rows of data is variable. The calculation to identify
  the rows that fall within the window could be very inefficient: the query
  will scan the entire dataset to find rows where the difference between
  cumulativeExpectedUpvotes and the latest cumulativeExpectedUpvotes falls
  within the window. So we save the samleTime of the start of the window in
  the database, so the query only needs to scan rows within this window.
*/
with parameters as (
    select 50 as windowSize
    , 2.3 as priorWeight
    , 0.003462767 as fatigueFactor
), latest as (
  select 
    latest.id
    , latest.sampleTime
    , latest.score 
    , latest.cumulativeUpvotes
    , latest.cumulativeExpectedUpvotes
    , ifnull(previous.upvoteRateWindow,0) as upvoteRateWindow
  from dataset latest join previousCrawl previous using (id)
  where latest.sampleTime = (select max(sampleTime) from dataset)
)
, windows as (
  select 
    latest.id
    , latest.sampleTime
    , latest.cumulativeUpvotes as cumulativeUpvotes
    , latest.cumulativeExpectedUpvotes as cumulativeExpectedUpvotes
    , max(dataset.sampleTime) as newWindow
    , min(latest.cumulativeUpvotes - dataset.cumulativeUpvotes) as upvotesInWindow
    , min(latest.cumulativeExpectedUpvotes - dataset.cumulativeExpectedUpvotes) as expectedUpvotesInWindow
    , min(latest.cumulativeExpectedUpvotes - dataset.cumulativeExpectedUpvotes) - windowSize as over
    , parameters.*
    from latest 
    join parameters
    left join dataset on
      latest.id = dataset.id 
      and dataset.sampleTime >= latest.upvoteRateWindow
      and latest.cumulativeExpectedUpvotes - dataset.cumulativeExpectedUpvotes > windowSize
  group by latest.id
)
update dataset
  set
    upvoteRate = case 
      when upvotesInWindow is null then ( dataset.cumulativeUpvotes + priorWeight ) / ( (1-exp(-fatigueFactor*dataset.cumulativeExpectedUpvotes))/fatigueFactor + priorWeight)
      else ( upvotesInWindow + priorWeight ) / (
          -- The formula for adjusting expected upvotes for fatigue comes from the assumption that expected upvote rate decays
          -- exponentially: fatigueAdjustedExpectedUpvoteRate = exp(-fatigueFactor*cumulativeExpectedUpvotes).
          -- So fatigueAdjustedExpectedUpvotes is the total area under this curve, or the integral of
          -- fatigueAdjustedExpectedUpvoteRate from 0 to max(cumulativeExpectedUpvotes), which is:
          --   ( 1-exp(-fatigueFactor*max(cumulativeExpectedUpvotes)) ) / fatigueFactor
          -- But no we want the area under the curve within the moving average window,
          -- So we integrate from max(cumulativeExpectedUpvotes) - expectedUpvotesInWindow to max(cumulativeExpectedUpvotes),
          -- which gives us the below formula.

          (
              exp(-fatigueFactor*(dataset.cumulativeExpectedUpvotes - expectedUpvotesInWindow))
              -exp(-fatigueFactor*dataset.cumulativeExpectedUpvotes)
          )/fatigueFactor
          + priorWeight)
    end
    , upvoteRateWindow = newWindow
from windows
where windows.id = dataset.id and windows.sampleTime = dataset.sampleTime;

-- select 
--   id
--   , sampleTime
--   , newWindow
--   , cumulativeUpvotes
--   , cumulativeExpectedUpvotes
--   , upvotesInWindow
--   , expectedUpvotesInWindow
--   , ( upvotesInWindow + priorWeight ) / ( expectedUpvotesInWindow + priorWeight) as movingAverageUpvoteRate
--   , ( cumulativeUpvotes + priorWeight ) / ( cumulativeExpectedUpvotes + priorWeight) as upvoteRate
-- from windows
-- where movingAverageUpvoteRate is not null
-- limit 10;




-- where datset.id = windows.id


-- select 
--   id
--   , newWindow
--   , cumulativeUpvotes
--   , cumulativeExpectedUpvotes
--   , upvotesInWindow
--   , expectedUpvotesInWindow
--   , ( upvotesInWindow + priorWeight ) / ( expectedUpvotesInWindow + priorWeight) as movingAverageUpvoteRate
--   , ( cumulativeUpvotes + priorWeight ) / ( cumulativeExpectedUpvotes + priorWeight) as upvoteRate
-- from windows join parameters
-- -- where movingAverageUpvoteRate is not null
-- limit 10;


