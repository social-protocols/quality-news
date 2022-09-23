# Quality News

Quality News reorders the Hacker News front page based on a ranking formula designed to promote more high "quality" stories. 

## Goal



Success on HN is partly a matter of luck. A few early upvotes can catapult a new story to the front page, resulting in a feedback loop of even more upvotes. But there are some very high quality stories don't ever get caught up in this feedback loop (thus the [second chance queue]). So there are false positives and false negatives: mediocre stories that got lucky, and high quality stories that didn't. We discussed this in our article on [Improving the Hacker News Ranking Formula].

We define "quality" like this: story A has a higher quality than story B, if story A **would** receive more upvotes than story B if it were shown at the same position on the HN front page. In other words, quality represents a "tendency to upvote": a measure of HN users' subjective preferences, but not an objective measure of how "good" a story is (Sometimes HN users upvote bad content simply because it makes for interesting conversation).


To compare the quality of different stories, we need to estimate how many upvotes each story **would have received** had they been shown on the same position (rank). To do so, we can look at historical upvote rates at each rank, and the ratios between them. For example, historically, stories at rank 2 (the second item on the page) get about 2/3 as many upvotes as stories at rank 1 (the first item on the page). So if a story received 2 upvotes at rank 2, we can estimate it would have received 3 upvotes at rank 1. 

Of course, do stories at rank 1 get more upvotes **because** they are at rank 1, or are the stories at rank 1 **because** they get more upvotes? Obviously, it is both. We need to untangle the causal effect of rank on upvotes from the effect of the ranking algorithm. [Todo: link to writeup]. Once we do this we can estimate how many upvotes the average story would receive **if** it were shown at a given rank.

## Attention

Stories with similar quality will often take a very different journey through Hacker News rankings, based on the randomness of early upvotes. We can track a story's history and count the total number of upvotes the average story would have received if it had the same history.

This number represents the amount of "attention" a story has received. We can then take the ratio upvotes/attention is an estimate of story quality. A story with a ratio greater than one has above average quality, because it has received more upvotes than the average story with the same history. A ratio less than one represents below-average quality.

	quality = upvotes / attention

## Bayesian Averaging

Of course, this ratio can still have a large component of luck. A perfectly average quality story can easily get 2 upvotes when only 1 is expected. So a simple ratio may be a poor estimate of quality when there is not a lot of data. A more sophisticated approach uses Bayesian inference: given our prior knowledge about the distribution of quality of the average story, plus the data we have about this particular story, what is the ideal Bayesian rational agent's estimate of the probable quality?

This question can be answered with a Bayesian hierarchical model. Using historical data we can generate prior beliefs about the quality distribution of the average story, and then use to calculate posterior estimates of the quality of each story, as described [here].

Once we have run the model, we can simplify the calculation using a technique called [Bayesian Averaging]. For example, if our prior belief about story quality is a Beta distribution with mean alpha/kappa, then the posterior after observing a sample of U upvotes and A units of attention will be approximately (U+alpha)/(A+kappa), which is actually a weighted average of the prior mean and the sample mean. Since the average story by definition has quality=1, the average is:

	( Upvotes + C ) / (Alpha + C)

Where the constant C represents the strength of the prior.

## Code

The application is a Go process running on a fly.io instance. 

The application crawls the [Hacker News API] every minute. For each story, we record the current rank and page (top, new, best, etc.), and how many upvotes it has received. The HN API has an endpoint that returns the IDS of all the stories on each page in order. But in order to get the current number of upvotes we need to make a separate API call for each story. We make several requests in parallel so that this is fast and represents a point-in-time "snapshot". For each story, we calculate how many upvotes the average story at that rank is expected to receive and update the accumulated attention for that story. The data is stored in a Sqlite database.

The frontpage generator queries the database and calculates the Bayesian average quality in the SQL query on the fly. It then uses the Go templating library to generate very HTML that mimic the original HN site.






