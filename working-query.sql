

-- with scores as (
--      select id, sampleTime, topRank,
--      pow(score-1, 0.8) / pow((sampleTime - submissionTime)/3600+2, 1.8) as score
--       , submissionTime
--       , score-1 as upvotes
--      from dataset
--      where topRank is not null
--      -- and sampleTime >= 1667244440
--      -- and sampleTime <= 1667244440 + 180

-- )
-- , penaltyFactors as (
--    select 
--       id
--       , sampleTime
--       , submissionTime
--       -- , resubmissionTime
--       , upvotes
--       , score
--       , topRank
--       , min(score) filter (where score > 0.1) over (partition by sampleTime order by topRank)  / score as maxPenaltyFactor
--       , min(score) filter (where score > 0.1) over (partition by sampleTime order by topRank) minScoreAbove
--     from scores
-- )

-- , maxPenaltyFactors as (
--    select 
--       id 
--       , sampleTime
--       , submissionTime
--       -- , resubmissionTime
--       , upvotes
--       -- , 1667244440+180-submissionTime as ageMinutes
--       , score
--       , topRank
--       , maxPenaltyFactor
--       , lag(maxPenaltyFactor,1) over (partition by id order by sampleTime) as lastMaxPenaltyfactor
--       , min(maxPenaltyFactor) over (partition by id order by sampleTime) as maxMaxPenaltyfactor
--       , lag(topRank, 1) over (partition by id order by sampleTime) as lastTopRank

--       , min(sampleTime) filter (where maxPenaltyFactor > 1) over (partition by id order by sampleTime) as resubmissionTime
--       -- , case when lag(topRank, 1) over (partition by id order by sampleTime) is null and score < 0.1 then sampleTime else null end as secondChanceTime
--       -- , min(case when score < 0.1 and topRank is not null then sampleTime else null end) over (partition by id order by sampleTime) as resubmissionTime
--       -- , min(sampleTime) over (partition by id order by sampleTime) as minSampleTime
--       from penaltyFactors
--     -- where ( sampleTime / 60 ) % 60 == 0 -- once an hour 
-- )
-- select * from maxPenaltyFactors where 
--  id = 33340610 order by sampleTime
-- limit 20;

-- , newScore as (
--    select 
--       * 
--       -- , pow(upvotes, 0.8) / pow((sampleTime - submissionTime)/3600+2, 1.8) as newScore
--       , pow(upvotes, 0.8) / pow((sampleTime - ifnull(resubmissionTime, submissionTime))/3600+2, 1.8)*ifnull(maxPenaltyFactor, 1) as newScoreWithPenalty
--       , min(pow(upvotes, 0.8) / pow((sampleTime - ifnull(resubmissionTime, submissionTime))/3600+2, 1.8)*ifnull(maxPenaltyFactor, 1)) filter (where score > 0.1) 
--          over (partition by sampleTime order by topRank)  / ( pow(upvotes, 0.8) / pow((sampleTime - ifnull(resubmissionTime, submissionTime))/3600+2, 1.8)*ifnull(maxPenaltyFactor, 1) ) as newMaxPenaltyFactor
--       -- , row_number() over () as rank
--    from maxPenaltyFactors
-- )
-- -- , newPenalty as (
-- --    select
-- --       *
-- --       , min(newScoreWithPenalty) filter (where score > 0.1) over (partition by sampleTime order by topRank)  / newScoreWithPenalty as newMaxPenaltyFactor
-- --       from newScore
-- -- )
--       -- where sampleTIme = 
--    -- where maxPenaltyFactor < 1.0
--    select * 
--    from newScore
--    where 
--       score > 0.1
--       and sampleTime = 1666796436
--       -- and resubmissionTime is not null
--       -- and newMaxPenaltyFactor > 1
--       -- and sampleTime = 1667244440 + 180
--    -- order by id, sampleTime desc
--    order by topRank
--    limit 30;


  -- where score > 0
  -- and sampleTime = (select max(sampleTime) from dataset)
  -- group by 1
  -- order by id, sampleTime desc
  -- limit 40;


    with parameters as (select 2.2956 as priorWeight, 2.2956 as overallPriorWeight, 1.75 as gravity),
       penalties as (
         select id, topRank, score as rankingScore, sampleTime, min(score) filter (where score > 0.1 and topRank is not null) over (partition by sampleTime order by topRank rows unbounded preceding)  / score as penaltyFactor
         from (
           select id, sampleTime, topRank,
           pow(score-1, 0.8) / pow((sampleTime - submissionTime)/3600+2, 1.8) as score
           from dataset
           where sampleTime = (select max(sampleTime) from dataset)
           and topRank is not null
         ) where score > 0.1
       )
select * from penalties 
where topRank < 60
;
  -- select
  --   id
  --   -- , by
  --   -- , title
  --   -- , url
  --   , submissionTime
  --   , cast(unixepoch()-submissionTime as real)/3600 as ageHours
  --   , score
  --   , descendants
  --   , penaltyFactor
  --   , (cumulativeUpvotes + priorWeight)/(cumulativeExpectedUpvotes + priorWeight) as quality 
  --   , topRank
  --   , qnRank
  -- from stories
  -- join dataset using(id)
  -- join parameters
  -- left join penalties using(id, sampleTime)
  -- where sampleTime = (select max(sampleTime) from dataset)
  -- order by 
  --   pow((cumulativeUpvotes + overallPriorWeight)/(cumulativeExpectedUpvotes + overallPriorWeight) * ageHours, 0.8) 
  --   / pow(ageHours+ 2, gravity) 
  --   -- * ifnull(penalties.penaltyFactor,1) 
  --   desc
  -- limit 10;



