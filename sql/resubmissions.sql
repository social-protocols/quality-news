-- ESTIMATING RESUBMISSION TIME

-- THE PROBLEM

-- When a story is resubmitted, its submission time is updated to the current
-- time, which gives it a rankings boost. 

-- We want to know what this new submission time is, so our algorithm can give
-- stories the same boost. Also our penalty calculate requires knowing each
-- story's pre-penalty ranking score, which requires knowing their submission
-- times. 

-- Unfortunately exact resubmission times are not currently published by HN. The API always
-- gives the story's original submission time.

-- Each story's submission time datestamp is also included in the HTML when
-- the story is displayed: you can see it when you hover the mouse over the
-- age field ("20 minutes ago").

-- Unfortunately, although the approximate age field age ("20 minutes ago:)
-- reflects the resubmission time, the datestamp in the HTML is the original
-- submission time.

-- So we can only estimate the resubmission time from this approximate age
-- field. 

-- But the approximate age is neither precise nor accurate. It is always a
-- whole number of minutes, hours, or days, rounded down: 1 hour 59 minutes
-- is show as "1 hour ago",  and 1 day 23 hours is shown as "one day ago".

-- When a story is less than an hour old, we have minute-level granularity,
-- However, this number is imprecise: it can be off by a couple of minutes
-- either way.

-- Further resubmitted stories don't seem to show up on the front page (at least
-- not the top 90 ranks we crawl) until they are at least an hour old.


-- THE SOLUTION We wrote dang to ask if he can help us out here. But I have
-- implemented a pretty accurate solution:

-- We can tell a story has been resubmitted within the last 24 hours because
-- the submission time will be far earlier (typically hours) than the
-- approximate age parsed from the web page (e.g. 3 hours ago). 

-- If the story is less than 1 day old, we can then place lower and upper
-- bounds on the resubmission time. If it says "3 hours", it means anyway
-- from 3:00 h to 3:59 h ago.

-- So each time we crawl, we calculate a lowe bound on the story's
-- resubmission time (based on an upper-boudn on age
-- , and then compare it to the previous upper bound and move the bound
--   accordingly (taking the lowest upper bound).

-- So if a story was submitted "3 hours ago" we know the story is at at most 4
-- hours old. So we save the sampleTime-4 hours in the submissionTime field,
-- understanding that this is a lower bound on submissionTime. Then in the
-- next minute we redo the calculation. If it still says "3 hours old" then
-- our new implied lower bound on submission time will be greater the the
-- previously lower bound by one minute. So we move up the lower bound up by a minue.
-- (lower bounds always move up as we discover higher lower bounds).

-- When the age string changes to "4 hours ago", we will know the story is at
-- least 5 hours 59 minutes old. But the implied submission time will one hour less
-- than the lower bound we calculated one minute before. So we keep the
-- current lower bound. At this point, we have the exact resumibssion time
-- within a couple of minutes either way.

-- Other considerations: We can't detect resubmission times for stories more than a day old
-- (unless they were resubmitted several days later) It is possible that a
-- resubmitted story is more than a day old, and is still on the front page.
-- In that case, we cannot determine it is a resubmitted story. So we need to
-- calculate the resubmission time beofre the stories is a day old. We then
-- remember this time, updating each subsequent datapoint to use this time.

with latest as (
  -- first, get the data from the latest crawl, determine which stories have
  -- been resubmitted, and estimate a lower bound on submission time
  select 
  *
  , timestamp as originalSubmissionTime
  , sampleTime - ageApprox - timestamp > 7200 and ageApprox < 3600*24 as resubmitted
  , cast(
    case 
      when 
        -- we know a story has been resubmitted if the submission time implied
        -- by the approximate age differs by too much. Because age is rounded
        -- down, the difference can be up to one hour plus a few minutes
        -- because of data delay. In practice, the difference is always
        -- several hours. Using a cutoff at two hours should be good. also,
        -- we should filter out stories more than a day old: if we just saw
        -- these stories for the first time, we don't know if they have been
        -- resubmitted or not (and thus don't know how old they really are)
        sampleTime - ageApprox - timestamp > 3600*2 and ageApprox < 3600*24
        and not job then
          -- calculate an upper bound on age
          case 
            when ageApprox < 3600 then ageApprox+59 -- e.g. if a story is "5 minutes old", it could be up to 5 minutes and 59 seconds old
            when ageApprox < 3600*24 then (ageApprox+59*60) -- if a story is "1 hour old" it could be up to 1:59m old
          end + 100 -- add another 100 seconds because the age field tends to be a little stale. 
        else sampleTime - timestamp
      end 
    as real) / 3600 as ageHours
  from dataset join stories using (id)
  where sampleTime = (select max(sampleTime) from dataset)
)
-- now get the data from the last crawl
, previous as (
  select 
    id
    , max(submissionTime) as submissionTime
  from dataset
  group by 1
)
update dataset as d 
-- And use the greater of the lower-bound submission time from the last crawl, and the one we just calculated.  
set submissionTime = case when latest.sampleTime - ageHours*3600 > ifnull(previous.submissionTime,0) then cast(latest.sampleTime - ageHours*3600 as int) else previous.submissionTime end
from latest
left join previous using (id)
where d.id = latest.id and d.sampleTime = latest.sampleTime;
