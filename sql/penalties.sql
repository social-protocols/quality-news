-- The idea of this penalty logic is to make the visible effect of penalties
-- similar to the effect on HN. If a story is demoted by several ranks on HN
-- it should be demoted several ranks on QN.

-- Demotion from rank 1 to rank 3 is more significant than demotion from rank
-- 81 to rank 83. So we'll measure penalties in terms of the story's actual
-- rank over its pre-penalty rank. So if a penalties causes a story to be
-- demoted from rank 1 to rank 3, the ratio is 3/1. The same if is is demoted
-- from rank 30 to rank 90.

-- The log of a ratio is a difference of logs. Wee compute penalty as 
-- the difference of logs, because if we take the average of the difference of
-- logs and convert back to a ratio, that is the same as a geometric average.

-- When the story is not in the top 90 the penalty ratio calculation is
-- tricky. If the pre-penalty rank is high then not showing up in the top 90
-- means the story has been highly penalized, but if the pre-penalty rank is
-- already close to 90 then it doesn't tell us much. 

-- The solution I am trying here is use the greater of the current moving
-- average and the previous moving average if the story is in the top 90.
-- So penalties can only increase. The exceptions are when there are less than
-- 30 minutes worth of data, in which case we use a weighted average of the 
-- moving average and zero (this is like Bayesian averaging with a prior of zero).
-- Also if the moving average falls consistently to negative territory, penalties
-- are removed.

with latestScores as ( 
  -- first, get the data from the latest crawl and calculate ranking scores
  select 
    *
    , score-1 as upvotes
    , pow(score-1, 0.8) / pow(cast(sampleTime - submissionTime as real)/3600+2, 1.8) as rankingScore -- pre-penalty HN ranking formula
    , submissionTime > timestamp as resubmitted
   from dataset join stories using (id)
   where sampleTime > (select max(sampleTime) from dataset) - 3600 -- look at last hour
   and score >= 3 -- story can't reach front page until score >= 3
), 
ranks as (
  select 
    *
    , ifnull(topRank,91) as rank
    , count(*) filter (where  ageApprox < 3600*24 and not job and upvotes > 0 and topRank is not null) over (partition by sampleTime order by rankingScore desc) as expectedRankFiltered
    , count(*) filter (where  ageApprox < 3600*24 and not job and upvotes > 0 and topRank is not null) over (partition by sampleTime order by topRank nulls last) as rankFiltered
  from latestScores
  order by rank
)
, movingAverages as (
  select 
    *
    , ifnull(
        avg(log(rankFiltered) - log(expectedRankFiltered)) filter(where rank > 3) 
          over (partition by id order by sampleTime rows between 59 preceding and current row) 
        , 0
      ) as movingAverageFilteredLogRankPenalty
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
    currentPenalty = log(rankFiltered) - log(expectedRankFiltered)
    , penalty =
      case 
        when numRows < 30 then
          -- If we have less than 60 values in our moving average window,
          -- calculate the moving average as if we had 60 values but the 
          -- missing values are zero. So the moving average will always start
          -- at zero and move up hopefully to a steady value after 30 minutes.
          case 
            when movingAverageFilteredLogRankPenalty > 0.1 then movingAverageFilteredLogRankPenalty * numRows / 30
            else 0
          end
        when rank <= 90 and movingAverageFilteredLogRankPenalty < 0 then
          -- Remove the penalty only if the log rank penalty has moved to be consistently negative
          -- which is strong evidence this story is no longer penalized.
          0
        else 
          -- Otherwise, use the greater of the previous penalty and the latest moving average.
          -- Set a threshold of 0.1 for applying penalties to remove some false positives.
          max(ifnull(previous.penalty,0), case
            when movingAverageFilteredLogRankPenalty > 0.1 then movingAverageFilteredLogRankPenalty
            else 0
          end)
      end
from latest
left join previousCrawl previous using (id)
where d.id = latest.id 
and d.sampleTime = latest.sampleTime;

