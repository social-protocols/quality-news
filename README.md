# About

[Quality News](https://social-protocols-news.fly.dev/) implements a new ranking formula for Hacker News designed to give stories the attention they deserve.

## Motivation

The success of a story on HN is partly a matter of timing and luck. A few early upvotes can catapult a new story to the front page where it can get caught in a feedback loop of even more upvotes. 

            →
        ↗       ↘
     rank     upvotes
        ↖       ↙
            ←


It is not always the best submissions that get caught in this feedback loop. We discussed this in our article on [Improving the Hacker News Ranking Algorithm](https://felx.me/2021/08/29/improving-the-hacker-news-ranking-algorithm.html).

This is the current hacker news ranking formula:

     rankingScore = pow(upvotes, 0.8) / pow(ageHours + 2, 1.8)

The problem is that it only considers 1) **upvotes** and 2) **age**. It doesn't consider 3) **timing** and 4) **rank**. So a story that receives 100 upvotes at rank 1 is treated the same as one that receives the same number of upvotes at rank 50. And upvotes received during peak hours US time are treated the same as upvotes received in the middle of the night. 

Our solution is to account for the effects of rank and timing, giving upvotes received at high ranks and peak times less weight.

## Upvote Share by Rank

We start by looking at historical upvotes for each rank for each page type (home/top, new, etc.). For example, the first story on the "top" page historically receives about 10.2% of all  upvotes (about 1.17 upvotes per minute), whereas the 40th story on the "new" page receives about %.05 of upvotes (about 0.0055 upvotes per minute).

We calculated this by [crawling the hacker news API](https://github.com/social-protocols/hacker-news-data) every minute for several months, and recording each story's rank and score. We then made some adjustments for the fact that stories may appear on more than one page during that minute. Here are the resulting numbers for the home (top) page:

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
## The Causal Model

But do stories at high ranks get more upvotes **because** they are shown at high ranks? Or are the stories shown at high ranks **because** they get more upvotes? Or both?

Here is an over-simplified hypothetical causal graph, based on some common sense assumptions:

          story      
        ↙       ↘
     rank   →  upvotes


Story is a confounding variable: it has a direct effect on upvotes, but because the ranking algorithm favors more up-votable stories, it also has a direct effect on rank. This means that even if rank had no direct effect on upvotes, there would be a correlation between rank and upvotes in the data!

Isolating the **direct causal effect** of rank on upvotes is important, because to properly adjust a story's score for rank, we need to know how many upvotes a story received **because** of the rank it was shown at, and how many it received **because** of how up-votable is is. 

We plan to publish a writeup on our approach to isolating the causal effect of rank on upvotes soon. 

Solving this problem tells us the share of upvotes that we would **expect** at each rank **if the ranking algorithm where completely random**.

## Expected Upvotes


So if we simply multiply expected upvote share at a rank by the total site-wide upvotes during some time interval, we get the number of upvotes **we would expect the average story to receive** at that rank during that time interval.

    expectedUpvotes[rank, time] = expectedUpvoteShare[rank] * sidewideUpvotes[time]

Given a history of the story's rank at each time, we can compute its total expected upvotes:

    totalExpectedUpvotes = sum{for each time} expectedUpvotes[rank[time], time] 

## The "True" Upvote Rate

We assume that each story has some "true" upvote rate, which is how much more or less likely users are to upvote that story than the average story. During each time interval, each story will receive on average the expected number of upvotes times its true upvote rate.

    upvotes[time] ≈ upvoteRate * expectedUpvotes[rank[time], time]

We assume that the relationship `upvotes ≈ upvoteRate * expectedUpvotes` holds even in the aggregate, regardless of the ranks at which upvotes actually occurred.

    totalUpvotes = sum{for each time} upvotes[time]
            = sum{for each time} upvoteRate * expectedUpvotes[rank[time], time]
            ≈ upvoteRate * sum{for each time} expectedUpvotes[rank[time], time]
            ≈ upvoteRate * totalExpectedUpvotes

Thus the **observed** upvote rate is an approximation of the true upvote rate:

    upvoteRate ≈ totalUpvotes / totalExpectedUpvotes


## Bayesian Averaging

But if we don't have a lot of data for a story, the observed upvote rate may be more a reflection of pure chance than of the true upvote rate.

A more sophisticated approach uses Bayesian inference: given our prior knowledge about the distribution of upvote rates, plus the evidence we have about this particular story, what does Bayes' rule tell us is the most probable true upvote rate?

Since the probability distribution in this case is continuous and complicated, Bayes rule actually can't be evaluated analytically using pen-on-paper math. Instead we run a Markov Chain Monte Carlo simulation in STAN on our Bayesian model to estimate the true upvote rate for each story given the data.

When we run this model we find that the true upvote rate estimates [**shrink**](https://www.statisticshowto.com/shrinkage-estimator/): they fall somewhere between the observed upvote rate (`totalUpvotes/totalExpectedUpvotes`) and 1.0. The more data we have for each story, the closer the estimate is to the observed upvote rate. 

In fact, the posterior is always just a weighted average of the observed upvote rate and the prior of 1.0. The weights are, respectively, the number of expected upvotes, and a constant representing the strength of the prior. If we know this constant, we can then estimate upvoteRate using the following formula -- a technique known as [Bayesian averaging](https://en.wikipedia.org/wiki/Bayesian_average).
      
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

## New Ranking Formula:

We can now substitute `hypotheticalUpvotes` into the HN ranking formula:

     newRankingScore = pow(hypotheticalUpvotes, 0.8) / pow(ageHours + 2, 1.8)
                     = pow(upvoteRate * age * c, 0.8) / pow(age + 2, 1.8)
                     = pow(c, 0.8) * pow(upvoteRate * age, 0.8) / pow(age + 2, 1.8)

We then drop the constant `pow(c, 0.8)` and substitute in our Bayesian average estimate of the upvote rate, to get our final ranking formula:

    newRankingScore = pow((totalUpvotes + weight) / (totalExpectedUpvotes + weight) * age, 0.8) / pow(age + 2, 1.8)



## Discussion: Expected Upvotes as Proxy for Attention

We expect more upvotes for stories shown at high rank during peak times because they receive more **attention**. Now we don't have any way to directly measure or even precisely define "attention" (we don't know what's going on in users's heads), but we know that the number of upvotes the average story receives must be roughly proportional to the amount of attention it receives (though there is a small attention fatigue factor). So expected Upvotes is a *proxy* for attention. 

With the current HN ranking formula, stories that receive a lot of early upvotes while the time penalty is still low can be ranked very high and thus receive more attention, which results in a feedback loop of even more upvotes (the rich get richer) until the quadratic age penalty finally dominates the ranking score. The effect of this feedback loop can overwhelm the effect of the story's true upvote rate.



               attention
                ↑     ↘
                +       +
                ↑         ↘
              rank ← + ← upvotes

Our proposed algorithm balances this feedback loop by giving expected upvotes -- our proxy for attention -- a direct negative effect on rank.

<!--
        expectedUpvotes
              ≈
          attention       true upvote rate 
        ↙   ↑     ↘       ↓                 
       -    +       +     +                                               
        ↘   ↑         ↘   ↓                
          rank ← + ← upvotes
-->



    expected               
     upvotes ≈ attention
         ↘      ↑     ↘
           -    +       +
             ↘  ↑         ↘
              rank ← + ← upvotes


So a story that gets a lot of upvotes early on will initially enjoy a higher rank and more attention, but this increased attention is a mixed blessing, because now the story is expected to receive more upvotes in proportion to the increased attention. In fact, the more initial success a story has, the quicker the negative penalty from expected upvotes will catch up to the benefits of additional attention. A story must have a high true upvotes rate among the average visitor to the Hacker News home page to sustain a high rank.

A large enough number of bots or colluding users can still distort the results. And many good stories will still be overlooked, because there are just too many stories: an above-average story needs several upvotes before there is enough information to overwhelm the weight of the prior assumption of average quality, but there are not necessarily enough people looking at the new page (thus the [second chance queue](https://news.ycombinator.com/item?id=11662380)) to provide these upvotes. We hope to experiment with a new reputation system that **rewards people for upvoting** new stories that prove to have a high true upvote rate. 

But we think overall this ranking formula should do a better job of giving stories the attention they deserve, reducing both over-ranked and under-ranked stories.

## Penalties for Off-Topic Stories

When we first built this, we immediately noticed a greater proportion of non-technical stories: mostly major news items from main-stream media that had little to do with hacking and startups. We reasoned that this was because Hacker News applies penalties to many main-stream news stories, but we didn't incorporate those penalties into our ranking algorithm. Here's a [blog post from 2013](https://www.righto.com/2013/11/how-hacker-news-ranking-really-works.html) that attempts to reverse-engineer these penalties


It is natural that, once a community has formed around some topic, they will come to want to discuss unrelated topics with that same community. Perhaps in the *short-term*, people gain the most value from discussing whatever topic interests them, but in the long term communities lose their value if they don't artificially focus the discussion. We believe HN has arrived at a compromise that quietly penalizes off-topic articles while still allowing them, so that the community remains focused on hacking and startups, but people also derive value from discussing other topics that truly interest them.

We are going to look into [inferring penalties and applying them](https://github.com/social-protocols/news/issues/47) on Quality News. But in the meantime, we think that's why you see more off-topic content.


# Development

The application is a single Go process that crawls the [Hacker News API](https://github.com/HackerNews/API) every minute. For each story, it records the current rank and page (top, new, best, etc.), and how many upvotes it has received, computes the attention share for that rank and updates the accumulated attention for that story. The data is stored in a Sqlite database.

The frontpage generator queries the database and calculates the Bayesian average upvote rate in the SQL query. It then uses the Go templating library to generate very simple HTML that mimics the original HN site. The frontpage is regenerated every minute and served compressed directly from memory.

## Running it locally

Make sure, you have
- go 1.18+
- [direnv](https://direnv.net/) - to set environment variables automatically

There is also a [shell.nix](shell.nix) available, which provides all required dependencies.


```sh
# if you don't have direnv installed:
source .envrc

go run *.go
```
