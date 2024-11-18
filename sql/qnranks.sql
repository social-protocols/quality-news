with parameters as (select %f as priorWeight, %f as overallPriorWeight, %f as gravity, %f as penaltyWeight, %f as fatigueFactor)
, latestData as (
	select	
		id
		, score
		, sampleTime
		, cast(sampleTime-submissionTime as real)/3600 as ageHours
		, cumulativeUpvotes
		, cumulativeExpectedUpvotes
		, penalty
	from dataset
	where sampleTime = (select max(sampleTime) from dataset)
	and score >= 3 -- story can't reach front page until score >= 3
  and coalesce(topRank, bestRank, newRank, askRank, showRank) is not null -- let's not rank stories if they aren't accumulating attention
),
qnRanks as (
  select 
  id
    , dense_rank() over(order by %s) as rank
    , sampleTime
    , penalty
  from latestData join parameters
)
update dataset as d set qnRank = qnRanks.rank
from qnRanks
where d.id = qnRanks.id and d.sampleTime = qnRanks.sampleTime;
