with rankingScores as (  
  select 
  	id 
    , sampleTime
    , topRank
    , pow(score-1, 0.8) / pow(cast(sampleTime - submissionTime as real)/3600+2, 1.8) as rankingScore -- pre-penalty HN ranking formula
    , ageApprox
    , job
    , score
    , timeStamp != submissionTime as resubmitted
  from dataset join stories using (id)
  where sampleTime = (select max(sampleTime) from dataset)
  -- normally a story is eligible to rank on front page once score >= 3 
  -- but jobs can be on the front  page without a score, and sometimes I see
  -- stories on the front page of a score of only 2. We want to calculate
  -- raw rank for any store that is ranked, or **should** be ranked.
  and (score >= 3 or topRank is not null)
  order by topRank asc, rankingScore desc
),
rawRanks as (
  select 
    id
    , sampleTime
    , job
    , resubmitted
    , topRank as rank
    , score
    , count(*) over (order by rankingScore desc) as rawRank
  from rankingScores 
  order by rank nulls last
)
update dataset as d
  set rawRank = count(*) over (
    order by case when rawRanks.job then rawRanks.rank else rawRanks.rawRank end, rawRanks.job desc
  )
  from rawRanks
  where d.id = rawRanks.id
  and d.sampleTime = rawRanks.sampleTime
;
