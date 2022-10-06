# About

[Quality News](https://social-protocols-news.fly.dev/) implements a new ranking formula for Hacker News designed to give stories the attention they deserve.

## Motivation

The success of a story on HN is partly a matter of timing and luck. A few early upvotes can catapult a new story to the front page where it can get caught in a feedback loop of even more upvotes. But many great submission don't ever get caught up in this feedback loop. We discussed this in our article on [Improving the Hacker News Ranking Algorithm](https://felx.me/2021/08/29/improving-the-hacker-news-ranking-algorithm.html).


This is the current hacker news ranking formula:

     rankingScore = pow(upvotes, 0.8) / pow(ageHours + 2, 1.8)

The problem is that it only considers 1) **upvotes** and 2) **age**. It doesn't consider 3) **timing** and 4)  **rank**. 


So a story that receives 100 upvotes at rank 1 is treated the same as one that receives the same number of upvotes at rank 50. And upvotes received during peak hours US time are the same as upvotes received in the middle of the night.

Each story makes a unique journey through the Hacker News site, spending different amounts of time on different ranks on different pages. To account for the effect of the story's history, our first step is to estimate the number of upvotes the average story would have received if it had the same history.

## Upvote Share by Rank

We start by looking at historical upvote rates for each rank for each page type. For example, the first story on the front page has historically received about 1.17 upvotes per minute, whereas the 40th story on the "new" page receives about 0.006 upvotes per minute.

Of course we should ask if stories at rank 1 get more upvotes **because** they are shown at rank 1, or are the stories shown at rank 1 **because** they get more upvotes? We've attempted to answer this question with a causal analysis, and our preliminary results indicate that it's primarily the former.

Once we establish the causal effect of rank on upvotes, we can calculate the expected average upvote *share* at each rank. Here's the results for the HN front page:

<img src="static/hn-top-page-upvotes-by-rank.png?raw=true" width="600">


<!--
Here are our calculations for a handful of ranks (for the front HN front page).

| rank  |  upvoteShare |
|-------|--------------|
| 1     |  0.10171544  |
| 2     |  0.06069524  |
| 3     |  0.04676849  |
| ...   |  ...         |
| 10    |  0.02380067  |
| ...   |  ...         |
| 50    |  0.00290519  |
| ...   |  ...         |
| 80    |  0.00110132  |

-->


So for example, roughly 10.2% of site-wide upvotes go to the first story on the home page, so `upvoteShare[1] = 10.2%`. The total share across all ranks adds up to 1.

## Expected Upvotes

The upvote share tells us how many upvotes a story would receive at a given rank as a percentage of the total number of site-wide upvotes. So if we simply multiply this by the site-wide upvotes during some time interval, we get the number of upvotes that we expect the average story to receive at that rank during that time interval.

    expectedUpvotes[rank, time] = upvoteShare[rank] * sidewideUpvotes[time]

Given a history of the story's rank at each time (given by `rank[time]`), we can compute its total expected upvotes:

    totalExpectedUpvotes = sum{for each time} expectedUpvotes[rank[time], time] 

## The "True" Upvote Rate

We assume that each story has some "true" upvote rate, which is how much more or less likely users are to upvote that story than the average story. During each time interval, each story will receive on average the expected number of upvotes times its upvote rate.

    upvotes[time] ≈ upvoteRate * expectedUpvotes[rank[time], time]

This relationship `upvotes ≈ upvoteRate * expectedUpvotes` holds even in the aggregate, regardless of the ranks at which upvotes actually occurred.

    totalUpvotes = sum{for each time} upvotes[time]
            = sum{for each time} upvoteRate * expectedUpvotes[rank[time], time]
            ≈ upvoteRate * sum{for each time} expectedUpvotes[rank[time], time]
            ≈ upvoteRate * totalExpectedUpvotes

Thus the aggregate upvote ratio is an approximation of the true upvote rate:

    upvoteRate ≈ totalUpvotes / totalExpectedUpvotes


## Bayesian Averaging

But if we don't have a lot of data for a story, the upvote ratio may be more a reflection of pure chance than of the true upvote rate.

A more sophisticated approach uses Bayesian inference: given our prior knowledge about the distribution of upvote rates, plus the evidence we have about this particular story, what does Bayes' rule tell us is the most probably true upvote rate?

Since the probability distribution in this case is continuous and complicated, Bayes rule actually can't be evaluated analytically using pen-on-paper math. Instead we run a Markov Chain Monte Carlo simulation in STAN on our Bayesian model to approximate the posterior distribution.

When we run this model we find that the posterior estimate of the true upvote rate for each story "shrinks": it falls somewhere between the upvote ratio for that story, and the average upvote ratio of 1. The more data we have for each story, the closer the posterior is to the actual upvote ratio. 

In fact, the posterior is always just a weighted average of the observed upvote rate and the prior of 1.0. The weights are, respectively, the number of expected upvotes, and a constant representing the strength of the prior. If we know this constant, we can then estimate upvoteRate using the following formula -- a technique known as Bayesian averaging. Our calculations are shown [here].
      
    U = totalUpvotes
    A = totalExpectedUpvotes
    W = weight of prior
                        
                     data     prior
                      ↓        ↓  
      upvoteRate ≈ ( U/A * A + 1 * W ) / (A + W) ≈ (totalUpvotes + weight) / (totalExpectedUpvotes + weight)
                           ↑       ↑        ↑
                         weight  weight   total
                           of      of     weight
                          data   prior 



## Hypothetical Upvotes

Now that we have an estimate for the true upvote rate for a story, we can estimate how many upvotes that story would have received if it had the same history as the average story. At each time interval, the average story received `sidewideUpvotes[time]/nStories` upvotes. So a story with a given upvoteRate would hypothetically have received:

    hypotheticalUpvotes = sum{for each time} upvoteRate * sidewideUpvotes[time]/nStories 
                        = upvoteRate * sum{for each time} * sidewideUpvotes[time]/nStories 
                        = upvoteRate * age * c

Where the constant `c` is the same for all stories. 

## Proposed new Ranking Formula:

We can now substitute `hypotheticalUpvotes` into the HN ranking formula:

     newRankingScore = pow(hypotheticalUpvotes, 0.8) / pow(ageHours + 2, 1.8)
                     = pow(upvoteRate * age * c, 0.8) / pow(age + 2, 1.8)
                     = pow(c, 0.8) * pow(upvoteRate * age, 0.8) / pow(age + 2, 1.8)

We then drop the constant `pow(c, 0.8)` and substitute in our Bayesian average estimate of the upvote rate, to get our final ranking formula:

    newRankingScore = pow((totalUpvotes + weight) / (totalExpectedUpvotes + weight) * age, 0.8) / pow(age + 2, 1.8)


## Discussion

We expect more upvotes for stories shown at high rank during peak times because they receive more **attention**. Now we don't have any way to directly measure or even precisely define "attention" (we don't know what's going on in users's heads), but we know the number of upvotes the average story receives must be roughly proportional to the amount of attention it receives (though there is a small attention fatigue factor). So expectedUpvotes is a *proxy* for attention. In fact in this code base, we use the term attention instead of expectedUpvotes.


With the current HN ranking formula, **early** upvotes are critical to a story's final score. Stories that receive a lot of upvotes for their age will be ranked higher and thus receive more attention, which results in a feedback loop of even more upvotes (the rich get richer) until the quadratic age penalty finally "catches up". Stories that don't receive these early upvotes can only catch up if they have a very high true upvote rate.

Our proposed algorithm fixes this. A story that gets a lot of upvotes early on will enjoy a higher rank for a while, but it will not be able to sustain that rank unless upvotes increase in proportion to the increase in attention. In fact, the more initial success a story has, the quicker the story will accumulate attention, and thus the sooner the score will approach the "true" upvote rate among HN users. Likewise stories that don't get lucky and receive a lot of  upvotes early on can catch up more easily, because they will also not have have accumulated much attention.

So this ranking formula theoretically gives stories the attention they "deserve": reducing both over-ranked and under-ranked stories (false-positives and false-negatives). 

Unfortunately, many good stories will still be overlooked, because there are just too many stories. A story needs several clicks in order to furnish enough data to overwhelm the weight of the prior assumption of average quality, but there not necessarily enough people looking at the new page (thus the [second chance queue](https://news.ycombinator.com/item?id=11662380)) to provide these clicks. We hope to experiment with a new reputation system that **rewards people for upvoting** and encourages early upvoting of new stories. 

# Development and Deployment

The application is a single Go process that crawls the [Hacker News API](https://github.com/HackerNews/API) every minute. For each story, it records the current rank and page (top, new, best, etc.), and how many upvotes it has received, computes the attention share for that rank and updates the accumulated attention for that story. The data is stored in a Sqlite database.

The frontpage generator queries the database and calculates the Bayesian average quality in the SQL query. It then uses the Go templating library to generate very simple HTML that mimics the original HN site. The frontpage is regenerated every minute and served compressed directly from memory.

## Hacking

Make sure, you have
- go 1.18+
- [direnv](https://direnv.net/) - to set environment variables automatically
(there is also a [shell.nix](shell.nix) available, to provide all the required dependencies)


```sh
# if you don't have direnv installed:
source .envrc

go run *.go
```
