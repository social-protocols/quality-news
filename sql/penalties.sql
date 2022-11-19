with latestScores as ( 
  -- first, get the data from the latest crawl and calculate ranking scores
  select 
    *
    , score-1 as upvotes
    , pow(score-1, 0.8) / pow(cast(sampleTime - timestamp as real)/3600+2, 1.8) as rankingScore -- pre-penalty HN ranking formula
    , submissionTime > timestamp as resubmitted
   from dataset join stories using (id)
   -- where sampleTime = 1668791580
   where sampleTime > (select max(sampleTime) from dataset) - 3600 -- look at last hours
   and score > 3 -- story can't reach front page until score > 3
), 
-- Now for each story (including those not on the front page) calculate the lowest pre-penalty ranking score of any
ranks as (
  select *
  , ifnull(topRank,91) as rank

  , count(*) filter (where not resubmitted and ageApprox < 3600*24 and not job and upvotes > 0 and topRank is not null) over (partition by sampleTime order by rankingScore desc) as expectedRankFiltered
  , count(*) filter (where not resubmitted and ageApprox < 3600*24 and not job and upvotes > 0 and topRank is not null) over (partition by sampleTime order by topRank nulls last) as rankFiltered

  from latestScores
  order by rank
)
, latest as (
  select 
    *
    , avg(log(rankFiltered) - log(expectedRankFiltered)) filter(where rank > 3) over (partition by id order by sampleTime rows between 59 preceding and current row) as movingAverageFilteredLogRankPenalty
  from ranks
)
update dataset as d
	set penalty = case
		when movingAverageFilteredLogRankPenalty > 0.5 then 1
		when movingAverageFilteredLogRankPenalty > 0.1 then 0.2
		else 0
	end
from latest
where d.id = latest.id and d.sampleTime = latest.sampleTime;