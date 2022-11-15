-- ESTIMATING PENALTY

-- Hacker News moderators apply latest to some stories for various reasons.
-- The story's ranking score is multiplied by the penalty factor. So the full
-- ranking formula, with latest, is: (upvotes^0.8)/(age+2)^1.8*penaltyFactor

-- We want to estimate these penalty factors and apply the same to our ranking
-- formula.

-- We can tell if there is a penalty because stories will be at the wrong rank
-- given their pre-penalty HN ranking score. If a story with a pre-penalty
-- ranking score of 1.5 is ranked below a story with ranking score of 1.0, it
-- means it must have been penalized by **at least** 1/3. Or in other words,
-- the penalty factor is less than 2/3.

-- This lower bound on the penalty is valid even if any of the higher-ranked
-- stories have been penalized. If story A with a score of 1.5 is ranked
-- below story B with a pre-penalty score of 1.0, and story B is also
-- penalized, it just means story A has a penalty even more than than 33%. 

-- So at each minute, we can estimate a lower bound on the penalty. Then we
-- can compare that bound to the previously calculated bound we take the
-- highest of the two lower bound.

-- To estimate the pre-penalty ranking score we need to now each story's age,
-- which means we need to know the resubmission time for resubmitted stories.

-- Penalties can also be removed. This is uncommon, but ideal we would be able
-- to detect this too.

-- So our algorithm for calculating an upper bound on the penalty factor is
-- simple: look at all stories ranked above the current story, find the one
-- with the **lowest* score. If this score is lower than the current story's
-- score, the current story must have been penalized, or wouldn't be ranked
-- lower. So the penaltyFactor is at most lowestScoreAbove /
-- currentStoryScore, so the penalty is at least (1 - lowestScoreOfStoriesAbove/currentStoryScore). 


-- Since we don't have very precise ages for resubmitted stories, we exclude
-- resubmitted stories when finding lowestScoreOfStoriesAbove, as well as
-- stories more than 24 hours old (because we don't know if they have been
-- resubmitted)

-- Because of data freshnesh issues, our calculations can be a bit off. If we
-- calculate a penalty factor of 98%, it actually probably means there is no
-- penalty and we don't quite have perfect data. So we round to the nearest 5%


with latestScores as ( 
  -- first, get the data from the latest crawl and calculate ranking scores
  select 
    *
    , score-1 as upvotes
    , pow(score-1, 0.8) / pow(cast(sampleTime - timestamp as real)/3600+2, 1.8) as rankingScore -- pre-penalty HN ranking formula
    , submissionTime > timestamp as resubmitted
   from dataset join stories using (id)
   where sampleTime = (select max(sampleTime) from dataset)
), 
-- Now for each story (including those not on the front page) calculate the lowest pre-penalty ranking score of any
lowestScores as (
  select *
  -- this is the key to estimating the penalty. It is the lowest pre-penalty
  -- ranking score of the stories ranked at or above the current story. the
  -- window includes the current story, so if there are no stories with a
  -- lower score above this, then this will give us the score of the current
  -- story.

  , min(rankingScore) filter (where not resubmitted and ageApprox < 3600*24 and not job and upvotes > 0 and topRank is not null) 
    over (partition by sampleTime order by topRank nulls last rows unbounded preceding) as lowestScoreAbove
  from latestScores
  order by topRank nulls last
)
-- now estimate a lower bound on the penalty
, latest as (
  select 
  *
  , case 
      when resubmitted or job or lowestScoreAbove > rankingScore then 0
      else 
        cast(cast(
          (1 - lowestScoreAbove / rankingScore) * 20 
        as int) as float)/20 -- round down to the nearest 5%.
    end as newPenalty
   from lowestScores
)
, previous as (
  select 
    id
    , max(penalty) as penalty
  from dataset
  group by 1
)
-- And use the greater of the lower penalty time from the last crawl, and the one we just calculated.  
update dataset as d 
set currentPenalty = newPenalty
    , penalty = max(ifnull(previous.penalty,0), newPenalty)
from latest
left join previous using (id)
where d.id = latest.id and d.sampleTime = latest.sampleTime;

