<h1 align="center" style="border-bottom: none">
    <div>
        <a href="https://news.social-protocols.org">
            <img src="static/logo.svg" width="80" />
            <br>
            Quality News
        </a>
    </div>
    Towards a fair ranking algorithm for Hacker News
</h1>

<div align="center">
 
![GitHub](https://img.shields.io/github/license/social-protocols/news?style=flat-square)

</div>



## About

[Quality News](https://news.social-protocols.org) is a [Hacker News](https://news.ycombinator.com) client that shows a new metric for stories called **upvoteRate**, which reflects the intent of the community.

**upvoteRate** is a measure of how much more or less likely users are to upvote a story than the average story. This makes the measure stable and comparable, regardless of of whether a story got caught in a feedback loop, was submitted at a different time or had a different number of users look at.

The **upvoteRate** calculation uses live minute-by-minute rank and upvote data collected from Hacker News.

It provides the following benefits compared to the raw upvote count:
- It's comparable, regardless of whether a story got caught in the feedback loop
- It's comparable across the time/day of week a story was submitted
- It's comparable across community sizes
- It reflects the intent of the community

With this project, we provide a frontpage, which looks and behaves very similar to the original Hacker News frontpage, but shows a few more metrics to better estimate voting behavior of the community. For now, the ranking is identical to Hacker News.

It's a fast, lightweight, server-side rendered page written in [go](https://go.dev) and hosted on [fly.io](https://fly.io).

## Motivation

The success of a story on HN is partly a matter of timing and luck. A few early upvotes can catapult a new story to the front page where it can get caught in a feedback loop of even more upvotes. 

```mermaid
graph LR
    R(Higher Rank)
    U(More Upvotes)
    R --> U
    U --> R
```

It is not always the best submissions that get caught in this feedback loop. We discussed some of our earlier thoughts on this problem in our article on [Improving the Hacker News Ranking Algorithm](https://news.ycombinator.com/item?id=28391659).

This is the current hacker news ranking formula:

     rankingScore = pow(upvotes, 0.8) / pow(ageHours + 2, 1.8)

The problem is that it only considers 1) **upvotes** and 2) **age**. It doesn't consider 3) **rank** or 4) **timing**. So a story that receives 100 upvotes at rank 1 is treated the same as one that receives 100 upvotes at rank 30. And upvotes received during peak hours are treated the same as upvotes received in the middle of the night.

Our goal is to provide a reliable metric, which can be used in the formula, replacing the raw upvote count. It accounts for the effects of rank and timing by giving upvotes received at high ranks and peak times less weight, eliminating the positive feedback loop.

This doesn't guarantee that some high quality stories won't sometimes be overlooked completely because nobody notices them on the new page. For those, we simply don't have enough data. We plan to approach this problem in the future.

## Upvote Share by Rank

We start by looking at historical upvote data on Hacker News for each rank and page type: `top` (front page), `new`, `best`, `ask`, and `show`. We obtained this data by [crawling the hacker news API](https://github.com/social-protocols/hacker-news-data) every minute for several months, and recording each story's rank and score (upvote count). The change in score tells us approximately how many upvotes occured at that rank.

We then calculated the *share* of overall site-wide upvotes that occur at each rank. For example, the first story on the `top` page receives on average about 10.2% of all upvotes (about 1.169 upvotes per minute), whereas the 40th story on the `new` page receives about 0.05% (about 0.0055 upvotes per minute). Upvote shares for the `top` page is summarized in the chart below.


<img src="static/upvote-share-by-rank.png" width="500" height="500">


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


## Expected Upvotes


If we multiply the upvote share for a rank by the total site-wide upvotes during some time interval, we get the number of expected upvotes for that rank and time interval. Or to be more precise, we get the number of upvotes **we would expect the average story to receive** at that rank during that time interval.

    expectedUpvotes[rank, timeInterval]
        = upvoteShare[rank] * sidewideUpvotes[timeInterval]

Given a history of the story's rank over time, we can compute its total expected upvotes:

    totalExpectedUpvotes
        = sum{for each timeInterval} expectedUpvotes[rank[timeInterval], timeInterval]


## The "True" Upvote Rate

We assume that each story has some "true" upvote rate, which is a factor of how much more or less likely users are to upvote that story than the average story. During each time interval, each story should receive, on average, the expected upvotes for the rank it was shown at times the story's true upvote rate:

    upvotes[timeInterval]
        ≈ upvoteRate * expectedUpvotes[rank[timeInterval], timeInterval]

The relationship `upvotes ≈ upvoteRate * expectedUpvotes` holds even in the aggregate, independently of the ranks at which upvotes actually occurred. This can be seen by taking the sum of a story's upvotes across all time intervals:

    totalUpvotes = sum{for each timeInterval} upvotes[timeInterval]
                 = sum{for each timeInterval} upvoteRate * expectedUpvotes[rank[timeInterval], timeInterval]
                 ≈ upvoteRate * sum{for each timeInterval} expectedUpvotes[rank[timeInterval], timeInterval]
                 ≈ upvoteRate * totalExpectedUpvotes

This means we can estimate the true upvote rate simply by dividing the story's total upvotes by its total expected upvotes:

    upvoteRate ≈ totalUpvotes / totalExpectedUpvotes

We call this estimate the **observed upvote rate**.

## Bayesian Averaging

If we don't have a lot of data for a story, the observed upvote rate may not be a very accurate estimate of the true upvote rate.

A more sophisticated approach uses Bayesian inference: given our prior knowledge about the distribution of upvote rates, plus the evidence we have observed about this particular story, what does Bayes' rule tell us is the most probable true upvote rate?

Since there are infinitely many possible true upvote rates, we can't use a trivial application of Bayes rule. But we can estimate the most likely true upvote rate using a technique called Bayesian Averaging. Here is good explanation of this technique from [Evan Miller](https://www.evanmiller.org/bayesian-average-ratings.html).

The Bayesian Average will be weighted average of the observed upvote rate (the data) and the average upvote rate of 1.0 (the prior). We weigh the observed upvote rate by the number of expected upvotes (which is roughly proportional to the number of people who have *paid attention* to a story and thus can be thought of as a proxy for sample size). We weigh the prior by a constant representing the strength of the prior (which can be thought of as the sample size necessary for the data to have more weight than the prior), which we estimated using an MCMC simulation.

The exact formula is shown below.

    U = totalUpvotes
    E = totalExpectedUpvotes
    W = weight of prior
                        
                             data     prior
                              ↓        ↓
     estimatedUpvoteRate ≈ ( U/E * E + 1 * W ) / (E + W)
                                   ↑       ↑        ↑
                                 weight  weight   total
                                   of      of     weight
                                  data   prior
                                                
                         ≈ (U + W) / (E + W)


## Possible Improvements


### The Causal Model

One improvement would be to properly account for **causality**. The formula for `estimatedUpvoteRate` makes some implicit assumptions about the causal relationship between the rank a story is shown at, and the number of upvotes it receives. These assumptions are not quite correct. As a result there are systematic errors in our upvote rate estimates that could be corrected by making appropriate statistical adjustments.

Just as more deaths occur in hospitals because society sends sick people to hospitals, and not necessarily because hospitals cause people to die, more upvotes occur at higher ranks because the HN ranking algorithm sends the highest-scoring stories to higher ranks. So when we look at the number of upvotes that historically occur at different ranks, we need to consider that this is due to the *combined* effect of the algorithm and the actual effect of rank on upvotes.

This is a problem because our upvote rate calculation depends on an estimate of the number of upvotes we would expect *the average story* to receive at each rank. But the data in our `upvoteShare`  table don't tell us this. Instead it tells us how many upvotes actually occured at each rank. But the *average* story was not shown at each rank. Above-average stories are generally shown at higher ranks, and below-average stories are generally shown at lower ranks.

So we don't know how many many more upvotes the average story *should* receive at rank 1 than at rank 90, just by looking at the historical averages.

Fortunately, there are statistical techniques for adjusting for these sorts of [confounding](https://en.wikipedia.org/wiki/Confounding) variables and estimating the direct effect of rank on upvotes. Applying these is tricky in this case, but we hope to update our algorithm with an updated `expectedUpvoteRate` table soon.

<!--
A story's upvote rate is by definition a factor of how many more or fewer upvotes a story gets than an average story would have gotten if shown at the same rank. For example, if a story received 3 upvotes during a minute at rank 1, then we need to divide 3 by the number of upvotes we would expect the average story to receive during a minute at rank 1. But we don't how many upvotes the average story gets during a minute at rank 1! Because the average story never makes it to rank 1; the historical upvotes at rank 1 only include data for above-average stories.
-->

### Fatigue

In general, upvote rates decrease as a story receives more attention. The more attention a story has received, the more likely it is that users have already seen it. So if a story spends a lot of time on  home page the upvote rate will eventually start to drop.

But we'd like a true upvote rate estimate that measures the tendency of the story itself to attract upvotes, and not the amount of attention it has received on Hacker News. We can do this by building a fatigue factor into the expected upvote calculation.

### Moving Averages

Looking only at more recent data could make vote manipulation even harder: it would require a constant supply of new votes as the moving average window moves.

The moving average window would be based on expected upvotes, not time, since expected upvotes can be thought of a proxy for sample size (as discussed in the [Bayesian Averaging](#bayesian-averaging) section above).

# Development

The application is a single Go process that crawls the [Hacker News API](https://github.com/HackerNews/API) every minute. For each story, it records the current rank and page (top, new, best, etc.), and how many upvotes it has received, computes the expected upvotes share for that rank and updates the accumulated expected upvotes for that story. The data is stored in a Sqlite database.

The frontpage generator queries the database and calculates the Bayesian average upvote rate in the SQL query. It then uses the Go templating library to generate HTML that mimics the original HN site. The frontpage is regenerated every minute and served compressed directly from memory.

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
