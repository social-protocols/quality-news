insert into attention
select id, max(score) as upvotes, max(submissionTime), 100, max(sampleTime)
from dataset
where id in (
	select distinct id from stories join dataset using (id)
)
group by id
limit 10;


