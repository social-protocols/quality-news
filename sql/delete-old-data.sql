-- delete data older than one month. It's taking about 18 days to fill a gig, and we have
-- 3 gig volume currently. That gives us 54 days. But 30 days is plenty and this gives us margin
-- in case the rate of growth of data increases.
delete from dataset left join votes on (id = storyID) where storyID is null and sampletime <= unixepoch()-30*24*60*60
