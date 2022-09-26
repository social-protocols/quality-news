# Quality News

Quality News reorders the Hacker News front page based on a ranking formula designed to promote more high "quality" stories. 

## Background

Success on HN is partly a matter of luck. A few early upvotes can catapult a new story to the front page, resulting in a feedback loop of even more upvotes. But there are some very high quality stories that don't ever get caught up in this feedback loop (thus the [second chance queue]). So there are false positives and false negatives: mediocre stories that got lucky, and high quality stories that didn't. We discussed this in our article on [Improving the Hacker News Ranking Formula].

### Quality

But what do we mean by "quality"? For our purposes, story A has a higher quality than story B, if story A **would** receive more upvotes than story B if it were shown at the same position on the HN front page. In other words, quality represents a "tendency to upvote": a measure of HN users' subjective preferences, but not an objective measure of how "good" a story is (in fact sometimes HN users upvote bad content simply because it makes for interesting conversation).


To compare the quality of different stories, we need to estimate how many upvotes each story **would have received** had they been shown on the same position (rank). To do so, we can look at historical upvote rates at each rank, and the ratios between them. For example, historically, the second story on the page get abouts 2/3 as many upvotes per unit time as the first story on the page. So if a story received 2 upvotes at rank 2, we can estimate it would have received 3 upvotes at rank 1. 

## Causality

Of course we should ask if stories at rank 1 get more upvotes **because** they are at rank 1, or are the stories at rank 1 **because** they get more upvotes? Obviously, it is both. We need to untangle the causal effect of rank on upvotes from the effect of the Hacker News ranking algorithm. [Todo: link to writeup]. Once we do this we can estimate how many upvotes the average story would receive **if** it were shown at a given rank.

## Attention

Stories with similar quality will often take a very different journey through Hacker News rankings, based partly on the randomness of early upvotes. Tracking a story's history and counting the total number of upvotes the average story with the same would have received tells us the amount of "attention" a story has received. 

    attention = sum(for each time t) rank[t]*expectedUpvotesAtRank[rank[t]]

We can then take the ratio upvotes/attention is an estimate of story quality. A story with a ratio greater than one has above average quality, because it has received more upvotes than the average story with the same history. A ratio less than one represents below-average quality.

    quality ≈ upvotes / attention

## Bayesian Averaging

Of course, this ratio can still have a large component of luck. A perfectly average quality story will often get 2 upvotes during the time when only 1 is expected. So a simple ratio may be a poor estimate of quality when there is not a lot of data. A more sophisticated approach uses Bayesian inference: given our prior knowledge about the distribution of quality of the average story, plus the evidence we have about this particular story, what does Bayes' rule tell us is the most probably quality?

Since the probability distribution in this case is continuous and complex, Bayes actually can't be evaluated "analytically" using pen-on-paper math. Instead we run a Markov Chain Monte Carlo simulation in STAN on our Bayesian model to approximate the posterior distribution as described [here]. 

When we run this model we find that the posterior estimate of quality for each story "shrinks": it falls somewhere between the ratio of upvotes/attention in the data we have for that story, and the average story quality of 1. The more data we have for each story, the closer the posterior is to upvotes/attention. In fact, the posterior is always just a weighted average of upvotes/attention and 1, with weights being the amount of attention and a constant representing the strength of the prior. If we know this constant, we can then estimate quality using the following formula -- a technique known as Bayesian averaging. Our calculations are shown [here].

      data     prior
       ↓        ↓  
    ( U/A × A + 1 × C ) / (A + C) = (U + C) / (A + C)
            ↑       ↑        ↑
          weight  weight   total
            of      of     weight
           data   prior 

## Code

The application is a Go process running on a fly.io instance. The code is open source [github.com/social-protocols/news].

The application crawls the [Hacker News API] every minute. For each story, it records the current rank and page (top, new, best, etc.), and how many upvotes it has received. The HN API has an endpoint that returns the IDs of all the stories on each page in order, but getting the current number of upvotes for each story requires making a separate API call for each story. The application makes these requests in parallel so that this is fast and represents a point-in-time "snapshot". For each story, it calculates how many upvotes the average story at that rank is expected to receive and updates the accumulated attention for that story. The data is stored in a Sqlite database.

The frontpage generator queries the database and calculates the Bayesian average quality in the SQL query onf the fly. It then uses the Go templating library to generate very HTML that mimic the original HN site.






