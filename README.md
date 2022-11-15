<h1 align="center" style="border-bottom: none">
    <div>
        <a href="https://news.social-protocols.org">
            <img src="static/logo.svg" width="80" />
            <br>
            Quality News
        </a>
    </div>
    A fair ranking algorithm for Hacker News
</h1>

<div align="center">
 
![GitHub](https://img.shields.io/github/license/social-protocols/news?style=flat-square)

</div>



## About

[Quality News](https://news.social-protocols.org) implements a new ranking formula for Hacker News, designed to give stories the attention they deserve.

The formula uses minute-by-minute rank and upvote data collected for every story on HN to adjust score based on the ranks and times at which upvotes occurred.

## Motivation

The success of a story on HN is partly a matter of timing and luck. A few early upvotes can catapult a new story to the front page where it can get caught in a feedback loop of even more upvotes. 

```mermaid
graph LR
    R(Higher Rank)
    U(More Upvotes)
    R --> U
    U --> R
```

It is not always the best submissions that get caught in this feedback loop. We discussed this in our article on [Improving the Hacker News Ranking Algorithm](https://felx.me/2021/08/29/improving-the-hacker-news-ranking-algorithm.html).

This is the current hacker news ranking formula:

     rankingScore = pow(upvotes, 0.8) / pow(ageHours + 2, 1.8)

The problem is that it only considers 1) **upvotes** and 2) **age**. It doesn't consider 3) **timing** or 4) **rank**. So a story that receives 100 upvotes at rank 1 is treated the same as one that receives 100 upvotes at rank 30. And upvotes received during peak hours US time are treated the same as upvotes received in the middle of the night.

Our solution is to account for the effects of rank and timing, giving upvotes received at high ranks and peak times less weight.

## Upvote Share by Rank

We start by looking at historical upvotes on Hacker News for each rank and page type (top/home, new, ask, etc.). For example, the first story on the "top" page receives on average about `10.2%` of all  upvotes (about `1.17` upvotes per minute), whereas the 40th story on the "new" page receives about `0.05%` of all upvotes (about `0.0055` upvotes per minute). We call this number `upvoteShare`.

We calculated `upvoteShare` for different ranks and page types by [crawling the hacker news API](https://github.com/social-protocols/hacker-news-data) every minute for several months, and recording each story's rank and score. We then made some adjustments for the fact that stories may appear on more than one page type during that minute.


<img src="static/hn-top-page-votehistogram.svg" width="400" style="float: right;">


<!--from the hacker-news-data database: 
    select 
        rank as topRank
        , round(avgUpvotes, 3) as avgUpvotes
        , round(avgUpvotes/(select sum(avgUpvotes) from upvotesByRank),3) as upvoteShare 
    from upvotesByRank 
    where rank in (1,2,3,10,40,80) and pageType = 'top';
-->


| topRank  | avgUpvotes   | upvoteShare |
| -------- | ------------ | ----------- |
| 1        | 1.169        | 10.2%       |
| 2        | 0.698        |  6.1%       |
| 3        | 0.538        |  4.7%       |
| ...      |              | ...         |
| 10       | 0.274        |  2.4%       |
| ...      |              | ...         |
| 40       | 0.043        |  0.4%       |
| ...      |              | ...         |
| 80       | 0.013        |  0.1%       |
| **TOTAL**| **11.493**   |  **100%**   |


## The Causal Model

But of course correlation does not always imply causation. The data above shows that stories at rank 1 get `1.169/0.698 = 67%` more upvotes than stories at rank 2. But does that mean that a story that got 100 upvotes at rank 2 would receive 167 upvotes at rank 1? No! 

Part of the reason we see more upvotes at rank 1 is that the algorithm places stories that get more upvotes at higher ranks. So even if rank had no causal effect on upvotes whatsoever, there would see a greater share of upvotes at higher ranks.

What we need to consider is the ratio of upvotes between ranks **for the same story**. When we do this, we find that stories at rank 1 receive on average only X% more upvotes than **the same story at rank 2**. 

Calculating this ratio for every rank gives us an estimate of the relative number of upvotes the same story would receive at different ranks. From this, we can estimate the share of upvotes we would **expect** the **average story** to receive at each rank.

We plan to publish another writeup on our approach to isolating the causal effect of rank on upvotes. 

## Expected Upvotes


So if we simply multiply the expected upvote share for a rank by the total site-wide upvotes during some time interval, we get the number of upvotes **we would expect the average story to receive** at that rank during that time interval.

    expectedUpvotes[rank, timeInterval]
        = expectedUpvoteShare[rank] * sidewideUpvotes[timeInterval]

Given a history of the story's rank over time, we can compute its total expected upvotes:

    totalExpectedUpvotes
        = sum{for each timeInterval} expectedUpvotes[rank[timeInterval], timeInterval]

This can be thought of as the number of upvotes we would expect a random (or average) story to receive if it had the same history.


## The "True" Upvote Rate

We assume that each story has some "true" upvote rate, which is a factor of how much more or less likely users are to upvote that story than the average story. During each time interval, each story will receive the on average expected number of upvotes times its true upvote rate.

    upvotes[timeInterval]
        ≈ upvoteRate * expectedUpvotes[rank[timeInterval], timeInterval]

We assume that the relationship `upvotes ≈ upvoteRate * expectedUpvotes` holds even in the aggregate, independently of the ranks at which upvotes actually occurred.

    totalUpvotes = sum{for each timeInterval} upvotes[timeInterval]
                 = sum{for each timeInterval} upvoteRate * expectedUpvotes[rank[timeInterval], timeInterval]
                 ≈ upvoteRate * sum{for each timeInterval} expectedUpvotes[rank[timeInterval], timeInterval]
                 ≈ upvoteRate * totalExpectedUpvotes

Thus the **observed** upvote rate is an approximation of the true upvote rate:

    upvoteRate ≈ totalUpvotes / totalExpectedUpvotes


## Bayesian Averaging

If we don't have a lot of data for a story, the observed upvote rate may be more a reflection of pure chance than of the true upvote rate.

A more sophisticated approach uses Bayesian inference: given our prior knowledge about the distribution of upvote rates, plus the evidence we have about this particular story, what does Bayes' rule tell us is the most probable true upvote rate?

Since the probability distribution in this case is continuous and complicated, Bayes rule actually can't be evaluated analytically using pen-on-paper math. Instead we run a Markov Chain Monte Carlo simulation in STAN on our Bayesian model to estimate the true upvote rate for each story given the data.

When we run this model we find that the true upvote rate estimates [**shrink**](https://www.statisticshowto.com/shrinkage-estimator/): they fall somewhere between the observed upvote rate (`totalUpvotes/totalExpectedUpvotes`) and 1.0. The more data we have for each story, the closer the estimate is to the observed upvote rate. 

In fact, the estimate is always just a weighted average of the observed upvote rate and the prior of 1.0. The weights are, respectively, the number of expected upvotes, and a constant representing the strength of the prior. If we know this constant, we can then estimate upvoteRate using the following formula -- a technique known as [Bayesian averaging](https://en.wikipedia.org/wiki/Bayesian_average).
      
    U = totalUpvotes
    A = totalExpectedUpvotes
    W = weight of prior
                        
                     data     prior
                      ↓        ↓  
      upvoteRate ≈ ( U/A * A + 1 * W ) / (A + W) 
                           ↑       ↑        ↑
                         weight  weight   total
                           of      of     weight
                          data   prior 
                                        
                 ≈ (U + W) / (A + W) 

<!--
TODO: why is A = weight of data?
//-->

## Hypothetical Upvotes

Now that we have an estimate for a story's true upvote rate, we can use it to create a ranking formula that rewards stories not just for receiving more upvotes, but for received more upvotes than **expected**. 

However, we cannot simply replace upvotes with the estimated upvote rate in the HN ranking formula. Here is the formula again:

     rankingScore = pow(upvotes, 0.8) / pow(ageHours + 2, 1.8)

`rankingScore` is ratio, where the numerator grows as a function of upvotes and the denominator grows as a function of age. If the numerator in our formula does not also grow similarly as a function of age, the result is an effectively stronger age penalty.

To make our formula as much like the HN formula as possible, the numerator should grow at exactly the same rate as it does in the current ranking formula for the average story, but it should grow at a faster (slower) rate for stories with above-average (below-average) true upvote rate.

We can accomplish this is instead of using the number of upvotes a story actually received, we estimate how many upvotes that story **would have received if it had the same history as the average story**. 

At each time interval, the average story received `sidewideUpvotes[timeInterval]/nStories` upvotes. So a story with a given upvoteRate would hypothetically have received:

    hypotheticalUpvotes
        = sum{for each timeInterval} upvoteRate * sidewideUpvotes[timeInterval]/nStories 
        = upvoteRate * sum{for each timeInterval} * sidewideUpvotes[timeInterval]/nStories 
        = upvoteRate * age * c

Where the constant `c` is the same for all stories. 

## New Ranking Formula:

We can now substitute `hypotheticalUpvotes` into the HN ranking formula:

     newRankingScore = pow(hypotheticalUpvotes, 0.8) / pow(ageHours + 2, 1.8)
                     = pow(upvoteRate * age * c, 0.8) / pow(age + 2, 1.8)
                     = pow(c, 0.8) * pow(upvoteRate * age, 0.8) / pow(age + 2, 1.8)

We then drop the constant `pow(c, 0.8)` and substitute in our Bayesian average estimate of the upvote rate, to get our final ranking formula:

    newRankingScore
        = pow(age * (totalUpvotes + priorWeight) / (totalExpectedUpvotes + priorWeight), 0.8) / pow(age + 2, 1.8)



## Discussion: Expected Upvotes as Proxy for Attention

We expect more upvotes for stories shown at high rank during peak times because they receive more **attention**. Now we don't have any way to directly measure or even precisely define "attention" (we don't know what's going on in users's heads), but we know that the number of upvotes the average story receives must be roughly proportional to the amount of attention it receives (though there is a small attention fatigue factor). So expected upvotes is a *proxy* for attention. 

With the current HN ranking formula, stories that receive a lot of early upvotes while the time penalty is still low can be ranked very high and thus receive more attention, which results in a feedback loop of even more upvotes (the rich get richer) until the quadratic age penalty finally dominates the ranking score. The effect of this feedback loop can overwhelm the effect of the story's true upvote rate.

```mermaid
graph LR
    A(Attention)
    U(Upvotes)
    R(Rank)
    R -->|+| A
    A -->|+| U
    U -->|+| R
```

Our proposed algorithm balances this feedback loop by giving expected upvotes -- our proxy for attention -- a direct negative effect on rank.

```mermaid
graph LR
    A("Expected Upvotes ≈ Attention")
    U(Upvotes)
    R(Rank)
    R -->|+| A
    A -->|+| U
    U -->|+| R
    A -->|-| R

    linkStyle 3 stroke:red;
```


So a story that gets a lot of upvotes early on will initially enjoy a higher rank and more attention, but this increased attention is a mixed blessing, because now the story is expected to receive more upvotes in proportion to the increased attention. In fact, the more initial success a story has, the quicker the negative penalty from expected upvotes will catch up. A story must have a high true upvote rate to sustain a high rank once it makes the front page.

A large enough number of bots or colluding users can still distort the results. And many good stories will still be overlooked, because there are just too many stories: an above-average story needs several upvotes before there is enough information to overwhelm the weight of the prior assumption of average quality, but there are not necessarily enough people looking at the new-page (thus the [second chance pool](https://news.ycombinator.com/item?id=11662380)) to provide these upvotes.

But we think overall this ranking formula should do a better job of giving stories the attention they deserve, reducing both over-ranked and under-ranked stories.

## Penalties for Off-Topic Stories

When we first built this, we immediately noticed a greater proportion of non-technical stories: mostly major news items from main-stream media that had little to do with ["hacking and startups"](https://news.ycombinator.com/newsguidelines.html). We reasoned that this was because Hacker News applies penalties to many main-stream news stories, but we didn't incorporate those penalties into our ranking algorithm. Here's a [blog post from 2013](https://www.righto.com/2013/11/how-hacker-news-ranking-really-works.html) that attempts to reverse-engineer these penalties


It is natural that, once a community has formed around some topic, they will come to want to discuss unrelated topics with that same community. Perhaps in the *short-term*, people gain the most value from discussing whatever topic interests them, but in the long term communities lose their value if they don't artificially focus the discussion. We believe HN has arrived at a compromise that quietly penalizes off-topic articles while still allowing them, so that the community remains focused on "hacking and startups", but people also derive value from discussing other topics that truly interest them.

We are going to look into [inferring penalties and applying them](https://github.com/social-protocols/news/issues/47) on Quality News. But in the meantime, we think that's why you see more off-topic content.


# Development

The application is a single Go process that crawls the [Hacker News API](https://github.com/HackerNews/API) every minute. For each story, it records the current rank and page (top, new, best, etc.), and how many upvotes it has received, computes the expected upvotes share for that rank and updates the accumulated expected upvotes for that story. The data is stored in a Sqlite database.

The frontpage generator queries the database and calculates the Bayesian average upvote rate in the SQL query. It then uses the Go templating library to generate very simple HTML that mimics the original HN site. The frontpage is regenerated every minute and served compressed directly from memory.

## Running it locally

Make sure, you have:

- go 1.19+
- [direnv](https://direnv.net/) - to set environment variables automatically
- entr - to automatically rerun server when files change
- sqlite3

Run:

```sh
git clone github.com/social-protocols/news
cd news

source .envrc # if you don't have direnv installed

go get
```

Then:

```sh
go run *.go
```

Or, to automatically watch for source file changes:

```sh
./watch.sh 
```

### Using NIX

There is also a [shell.nix](shell.nix) available, which provides all required dependencies.

Install nix on your system, enter the news directory, and run:      

```sh 
    git clone github.com/social-protocols/news
    cd news
    nix-channel --update
    nix-shell
    ./watch.sh
```

# Contributions

All contributions are welcome! Please open issues and PRs.
