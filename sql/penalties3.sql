with latestScores as ( 
  -- first, get the data from the latest crawl and calculate ranking scores
  select 
    *
    , ifnull(topRank, 91) as rank
    , score-1 as upvotes
    , pow(score-1, 0.8) / pow(cast(sampleTime - submissionTime as real)/3600+2, 1.8) as rankingScore -- pre-penalty HN ranking formula
    , submissionTime > timestamp as resubmitted
   from dataset join stories using (id)
--   where sampleTime > (select max(sampleTime) from dataset) - 3600
   where sampleTime = (select max(sampleTime) from dataset)
   and score > 3
) 

select 
  a.id
  , sum(case when a.rankingScore > b.rankingScore then 1 else 0 end)
  
  -- , max(a.rank) as maxRank
  -- , min(a.rank) as minRank
  -- , max(a.job) job
  -- , max(a.resubmitted) resubmitted
  -- , avg(a.rankingScore) as avgRankingScore

--  , case when a.rankingScore > b.rankingScore then b.rankingScore/a.rankingScore else 1 end + 30 / (count(*) + 30) as s

  -- , sum(case when a.rankingScore > b.rankingScore then b.rankingScore/a.rankingScore else 1 end) as sumPenalty
  -- , min(case when a.rankingScore > b.rankingScore then b.rankingScore/a.rankingScore else 1 end) as minPenalty
  -- , avg(case when a.rankingScore > b.rankingScore then b.rankingScore/a.rankingScore else 1 end) as avgPenalty
  -- , count(*) as c

  -- , ( sum(case when a.rankingScore > b.rankingScore then b.rankingScore/a.rankingScore else 1 end) + 30 ) / (count(*) + 30) as p1

  -- , sum(case when a.rankingScore > b.rankingScore then 1 else 0 end)
  -- , count(*) as c2
  -- , cast(sum(case when a.rankingScore > b.rankingScore then 1 else 0 end) as real)/count(*) as p2
from latestScores a join latestScores b on (
  a.sampleTime = b.sampleTime
  and a.rank > b.rank
  and not b.resubmitted and b.ageApprox < 3600*24 and not b.job and b.upvotes > 0
)
group by a.id
-- having ifnull(rank,91) > 60
--having a.id = 33627969
order by maxRank nulls last
limit 100;
  


