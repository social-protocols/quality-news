-- The idea of this penalty logic is to make the effect of penalties similar
-- to the effect on HN. If a story is demoted by several ranks on HN it
-- should be demoted several ranks on QN.
--
-- We actually calculate penalty as a ratio topRank/rawRank. We take a
-- geometric moving average of this ratio. To apply the penalty, we multiply
-- qnRank by this ratio.
--
-- Because we only crawl the top 90 stories, then if the rank is >= 90 we
-- don't actually know the penalty.


with parameters as ( select 30 as movingAverageWindowLength
    , .01 as penaltyThreshold
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
    , count(*) filter (where ageApprox < 3600*24 and topRank is not null) over (partition by sampleTime order by rankingScore desc) + 1 as rawRankFiltered
    , count(*) filter (where ageApprox < 3600*24 and topRank is not null) over (partition by sampleTime order by topRank nulls last) + 1 as rankFiltered
  from latestScores
  order by rank
)
, movingAverages as (
  select 
    *
    , 
      ifnull(
        avg( log(rankFiltered/rawRankFiltered) )
          filter (where topRank is not null) 
          over (partition by id order by sampleTime rows between 59 preceding and current row) 
        , 0
      ) 
      as movingAverageFilteredRankPenalty
    , ifnull(
        count(*)
          filter (where topRank is not null) 
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
    currentPenalty = case when latest.score < 4 then 0 else log(rankFiltered/rawRankFiltered) end
    , penalty =
      case 

        -- story can't reach front page until score >= 3. But I have observed
        -- that sometimes it takes score reaching 4. Unless story is eligible
        -- to reach front page we can't estimate penalties. But we can apply
        -- a default domain penalty.
        when latest.topRank is null then ifnull(previous.penalty,0)
        when latest.score < 4 then ifnull(domain_penalties.avg_penalty,0)
        when abs(movingAverageFilteredRankPenalty) > movingAverageFilteredRankPenalty then movingAverageFilteredRankPenalty
        else ifnull(previous.penalty,0) 
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

