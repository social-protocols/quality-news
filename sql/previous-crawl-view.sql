-- This query selects the previous datapoint for every story in the latest crawl
-- It is a bit tricky because the sampleTime may be different for each story, because
-- Some stories may appear and disappear from crawl results if they fall off the front page and reappear.

create temporary table previousCrawl as
with latest as (
  select * from dataset
  where sampleTime = (select max(sampleTime) from dataset)
)
-- identify stories that are in the previous crawl. This is a quick indexed lookup
, previousCrawl as (
  select
    id
    , sampleTime
  from dataset
  where sampleTime = (select max(sampleTime) from dataset where sampleTime != (select max(sampleTime) from dataset))
)
-- this this query finds the sampleTime of the last time this story was
-- crawled, for all stories that were not in the previous crawl. This
-- subquery can be slow, so only do it for stories that weren't in the
-- previous crawl.
, previousSampleForStory as (
  select
    latest.id
    , ifnull(previousCrawl.sampleTime, max(dataset.sampleTime)) as sampleTime
    , previousCrawl.sampleTime is null as gapInData
  from latest left join previousCrawl using (id)
  left join dataset on (
    previousCrawl.id is null
    and latest.id = dataset.id
    and dataset.sampleTime < (select max(sampleTime) from dataset)
  )
  group by 1
)
select dataset.*, gapInData from previousSampleForStory join dataset using (id, sampleTime);
