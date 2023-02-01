-- The idea of this penalty logic is to make the visible effect of penalties
-- similar to the effect on HN. If a story is demoted by several ranks on HN
-- it should be demoted several ranks on QN.

-- Here we are experimenting with defining penalties in terms of the absolute
-- difference in ranks. So if the story is expected to be at rank 2 and is at
-- rank 5, then the penalty is 5-2-3.

-- Because we only crawl the top 90 stories, then if the rank is >= 90 we
-- don't actually know the rank. This puts a cap on the penalty estimate.
-- When the story rank is close to 90, then this will tend to result in
-- underestimated penalties.

-- The solution I am trying here is use the greater of the current moving
-- average and the previous moving average if the story is in the top 90. So
-- penalties can only increase. The exceptions are when there are less than
-- movingAverageWindowLength minutes worth of data, in which case we use a
-- weighted average of the moving average and the domain-level default
-- penalty (this is like Bayesian averaging with the domain penalty as
-- prior). Also if the moving average falls consistently to negative
-- territory, penalties are removed.

with parameters as ( select 30 as movingAverageWindowLength
    , 2 as penaltyThreshold
),
latestScores as (
  -- first, get the data from the latest crawl and calculate ranking scores
  select 
    *
    , score-1 as upvotes
    , pow(score-1, 0.8) / pow(cast(sampleTime - submissionTime as real)/3600+2, 1.8) as rankingScore -- pre-penalty HN ranking formula
    , submissionTime > timestamp as resubmitted
   from dataset join stories using (id)
   where sampleTime > (select max(sampleTime) from dataset) - 3600 -- look at last hour
), 
ranks as (
  select 
    *
    , ifnull(topRank,91) as rank
    , count(*) filter (where ageApprox < 3600*24 and topRank is not null) over (partition by sampleTime order by rankingScore desc) as rawRankFiltered
    , count(*) filter (where ageApprox < 3600*24 and topRank is not null) over (partition by sampleTime order by topRank nulls last) as rankFiltered
  from latestScores
  order by rank
)
, movingAverages as (
  select 
    *
    , ifnull(
        avg(rankFiltered - rawRankFiltered)
          over (partition by id order by sampleTime rows between 59 preceding and current row) 
        , 0
      ) as movingAverageFilteredRankPenalty
    , ifnull(
        count(*)
          over (partition by id order by sampleTime rows between 59 preceding and current row) 
        , 0
      ) as numRows
  from ranks
)
, latest as (
  select * from movingAverages
  where sampleTime = (select max(sampleTime) from dataset)
)
update dataset as d
  set 
    currentPenalty = case when latest.score < 4 then 0 else rankFiltered - rawRankFiltered end
    , penalty =
      case 

        -- story can't reach front page until score >= 3. But I have observed
        -- that sometimes it takes score reaching 4. Unless story is eligible
        -- to reach front page we can't estimate penalties. But we can apply
        -- a default domain penalty.
        when latest.score < 4 then ifnull(domain_penalties.avg_penalty,0)
        when numRows < movingAverageWindowLength and previous.penalty == 0 then
          -- If we have less than movingAverageWindowLength values in our moving average window,
          -- and we don't have a previous penalty, then 
          -- calculate the moving average as if we had movingAverageWindowLength values but the
          -- missing values are equal to the default domain penalty. So the moving average will always start
          -- at the domain penalty and move hopefully to a steady value after movingAverageWindowLength minutes.
          case 
            when abs(movingAverageFilteredRankPenalty) > penaltyThreshold then
              ( movingAverageFilteredRankPenalty * numRows  + ifnull(domain_penalties.avg_penalty,0) * (movingAverageWindowLength - numRows) ) / movingAverageWindowLength
            else 0
          end
        when movingAverageFilteredRankPenalty < 0 then
          -- If we have a negative penalty (a boost), then use the greater (in terms of absolute value) of the previous penalty and the current negative penalty
          -- Note if previous penalty is positive, but the moving average is now negative, then the penalty will be either be 1) changed to a boost 
          -- or 2) removed completely if the absolute value of the new moving average isn't above the threshold, or if the story doesn't rank <= 90 (a negative
          -- penalty calculation for a story at rank 91 is meaningless, since 91 isn't a real rank).
          min(ifnull(previous.penalty,0), case
            when abs(movingAverageFilteredRankPenalty) > penaltyThreshold and rank <= 90 then movingAverageFilteredRankPenalty
            else 0
          end)
        else
          -- Otherwise, use the greater of the previous penalty and the latest moving average.
          -- Set a threshold of penaltyThreshold for applying penalties to remove some false positives.
          max(ifnull(previous.penalty,0), case
            when movingAverageFilteredRankPenalty > penaltyThreshold then movingAverageFilteredRankPenalty
            else 0
          end)
      end
from latest
left join previousCrawl previous using (id)
left join domain_penalties on (
  url like ('http://' || domain || '%')
  or url like ('https://' || domain || '%')
)
join parameters
where d.id = latest.id 
and d.sampleTime = latest.sampleTime;

