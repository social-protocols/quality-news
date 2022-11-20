with parameters as (select %f as priorWeight, %f as overallPriorWeight, %f as gravity, %f as penaltyWeight)
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
	and score > 3
),
uadjustedranks as (
	select 
	id
		, dense_rank() over(order by %s) as unadjustedRank
		, sampleTime
		, penalty
	from latestData join parameters
)
qnRanks as (
	select 
	id
		, dense_rank() over(order by unadjustedRank*power(10,penalty) desc) as rank
		, sampleTime
	from unadjustedRank
)
update dataset as d set qnRank = qnRanks.rank
from qnRanks
where d.id = qnRanks.id and d.sampleTime = qnRanks.sampleTime;
