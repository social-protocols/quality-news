with parameters as (
  select
    1.50 as priorWeight
    , 0.003462767 as fatigueFactor
), 
stories as (
  select
    id 
    , votes.entryTime is not null as mystory
    , entryUpvoteRate
    , max(cumulativeUpvotes) as cumulativeUpvotes
    , max(cumulativeExpectedUpvotes) as cumulativeExpectedUpvotes
    , max(score) as score
    , (cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight) qualityScore

    , log((cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight))*100 gain


  from dataset
  join parameters
  left join votes on
    votes.userID = 1
    and votes.storyID = dataset.id


  -- where id >= (select min(storyID) from votes where userID = 1 and storyID > 36754601) and id <= (select max(storyID) from votes where userID = 1 and storyID > 36754601) 
  -- where id >= (select min(storyID) from votes where userID = 1 and storyID > 36780531) and id <= (select max(storyID) from votes where userID = 1 and storyID > 36780531) 
  -- where id >= (select min(storyID) from votes where userID = 1) 
where id >= (select min(storyID) from votes where userID = 1) and id <= (select max(storyID) from votes where userID = 1)

  -- and id <= (select max(storyID) from votes where userID = 1) 

  group by id
)

-- select * from stories where id = 36805284; 



, sums as (
    select
    sum(case when mystory then cumulativeUpvotes else null end) as myCumulativeUpvotes
    , sum(case when mystory then cumulativeExpectedUpvotes else null end) as myCumulativeExpectedUpvotes
    , avg(case when mystory then score else null end) as myAverageScore
    , avg(case when mystory then cumulativeUpvotes / cumulativeExpectedUpvotes else null end) as myAverageUpvoteRate

    -- The below doesn't make sense. Because cumulativeUpvotes are sometimes 0, and the log of 0 is not defined.
    -- , exp(avg(case when mystory then log(cumulativeUpvotes / cumulativeExpectedUpvotes) else null end)) as myGeoAverageUpvoteRate


    -- , sum(case when votes.entryTime is not null then score-1 else null end)/count(distinct votes.storyID) as myAverageScore
    , sum(cumulativeUpvotes) as overallCumulativeUpvotes
    , sum(cumulativeExpectedUpvotes) as overallCumulativeExpectedUpvotes
    , avg(score) as overallAverageScore
    , avg(cumulativeUpvotes / cumulativeExpectedUpvotes) as overallAverageUpvoteRate

    -- The below doesn't make sense. Because cumulativeUpvotes are sometimes 0, and the log of 0 is not defined.
    -- , exp(avg(log(cumulativeUpvotes / cumulativeExpectedUpvotes))) as overallGeoAverageUpvoteRate


    , exp(avg(log((cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight)))) geoAverageQualityScore


    , sum(log((cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight))   )*100 baselineGain


    -- , exp(avg(log((cumulativeUpvotes + priorWeight)/(cumulativeExpectedUpvotes + priorWeight)))) geoAverageQualityScore


    -- , sum(case when votes.entryTime is null then score-1 else null end)/(count(distinct dataset.id) - count(distinct votes.storyID)) as overallAverageScore    
    from stories
    join parameters
)
select 
  -- *
  myAverageScore
  , myAverageUpvoteRate
  , myCumulativeUpvotes/myCumulativeExpectedUpvotes as myUpvoteRate
  , overallAverageScore
  , overallAverageUpvoteRate
  , overallCumulativeUpvotes/overallCumulativeExpectedUpvotes as overallUpvoteRate
  , geoAverageQualityScore
  , baselineGain
from sums;


-- Discussion: The geomean quality score is close to 1, as expected. The average score is greater than 1, because that's what will happen
-- if you take the average of exp(x) when the average of x is 0. FOr example in R:
-- (ins)> x = rnorm(10000, mean=0, sd=2)
-- (ins)> mean(x)
-- [1] -0.007797868
-- (ins)> mean(exp(x))
-- [1] 9.844065
