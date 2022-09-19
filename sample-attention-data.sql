insert into attention
select id, score as upvotes, submissionTime, 100, sampleTime
from dataset 
where id in (
	select distinct id from stories join dataset using (id)
)
limit 10;


