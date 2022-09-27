# Quality News

Quality News implements a new ranking formula for Hacker News designed to give stories the attention they deserve.

## Motivation

Success on HN is partly a matter of timing and luck. A few early upvotes can catapult a new story to the front page, resulting in a feedback loop of even more upvotes. But many great submission don't ever get caught up in this feedback loop. We discussed this in our article on [Improving the Hacker News Ranking Algorithm](https://felx.me/2021/08/29/improving-the-hacker-news-ranking-algorithm.html).

The problem is that the ranking algorithm only considers 1) **upvotes** and 2) **age**. It doesn't consider 3) **timing** and 4)  **rank**. For example, a story that receives 100 upvotes at rank 1 at midday US time should not have the same ranking score as one that receives the same number of upvotes at rank 50 at midnight. 



Each story makes a unique journey through the Hacker News site, spending different amounts of time on different ranks on different pages. So our first step is to estimate the number of upvotes the average story would have received if it had the same history.

## Upvote Share

We start by look at historical upvote rates for each rank for each page type. For example, the first story on the front page has historically received about 1.18 upvotes per minute. So if a story spends 2 minutes at rank 1, we would expect 2.36 upvotes.

Of course we should ask if stories at rank 1 get more upvotes **because** they are shown at rank 1, or are the stories shown at rank 1 **because** they get more upvotes? Obviously, it is both. We need to untangle the causal effect of rank on upvotes from the effect of the algorithm. [Todo: link to writeup]

Once we do this, we can calculate the expected average upvote *share* at each rank. For example, we have found that on average, roughly 10% of sitewide upvotes go to the first story on the home page, so `upvoteShare[1] = 10%`.


## Attention

The upvote share tells us how many upvotes a story would receive at a given rank as a percentage of the total number of sitewide upvotes. So if we simply multiply this by the total number of sitewide upvotes at time t, we get the approximate number of upvotes the average story would have received during time interval t at rank r.


This estimate can be thought of as a proxy for the amount of **attention** a story has received.

    attention[r, t] = upvoteShare[r] * sidewideUpvotes[t]

### Upvote Ratio

Given a history of the story's rank at each time t, we can compute the total amount of attention a story has received:

    attention = sum{t} attention[rank[t], t] 

And compare this to the upvotes the story has actually received:

    upvoteRatio = upvotes / attention

## The "True" Upvote Rate

We assume that each story has some "true" upvote rate, which is how much more or less likely users are to upvote that story than the average story. 

    upvotes[t] ≈ upvoteRate * attention[t, rank[t]]


The average story has an upvote rate of 1. 

In the long run, the observed upvote ratio will approximate the true upvote rate. This is true regardless of the ranks at which upvotes occurred!

    upvotes = sum{t} upvotes[t]
            ≈ sum{t} upvoteRate * attention[t, rank[t]]
            ≈ upvoteRate * sum{t} attention[t, rank[t]]
            ≈ upvoteRate * attention
So:

    upvoteRate ≈ upvotes / attention

## Bayesian Averaging

But if we don't have a lot of data for a story, the upvote ratio may be more a reflection of pure chance than of the true upvote rate.

A more sophisticated approach uses Bayesian inference: given our prior knowledge about the distribution of upvote rates, plus the evidence we have about this particular story, what does Bayes' rule tell us is the most probably true upvote rate?

Since the probability distribution in this case is continuous and complex, Bayes rule actually can't be evaluated analytically using pen-on-paper math. Instead we run a Markov Chain Monte Carlo simulation in STAN on our Bayesian model to approximate the posterior distribution as described [here]. 

When we run this model we find that the posterior estimate of the true upvote rate for each story "shrinks": it falls somewhere between the upvote ratio for that story, and the average upvote ratio of 1. The more data we have for each story, the closer the posterior is to the actual upvote ratio. 

In fact, the posterior is always just a weighted average of the observed upvote rate and the prior of 1.0. The weights are, respectively, the amount of attention and a constant representing the strength of the prior. If we know this constant, we can then estimate upvoteRate using the following formula -- a technique known as Bayesian averaging. Our calculations are shown [here].
      
    U = upvotes
    A = attention
    W = weight of prior
                        
                     data     prior
                      ↓        ↓  
      upvoteRate ≈ ( U/A * A + 1 * W ) / (A + W) ≈ (upvotes + W) / (attention + W)
                           ↑       ↑        ↑
                         weight  weight   total
                           of      of     weight
                          data   prior 



## Hypothetical Upvotes

Now that we have an estimate for the true upvote rate for a story, we can estimate how many upvotes that story would have received if it had the same history as the average story. 

    hypotheticalUpvotes = sum{t} upvoteRate * sidewideUpvotes[t]/nStories 
                        = upvoteRate * sum{t} * sidewideUpvotes[t]/nStories 
                        = upvoteRate * age * c

Where c is the same for all stories. 

## Proposed new Ranking Formula:

We can now substitute hypotheticalUpvotes into the HN ranking formula:

     newRankingScore = pow(hypotheticalUpvotes, 0.8) / pow(ageHours + 2, 1.8)
                     = pow(upvoteRate * age * c, 0.8) / pow(age + 2, 1.8)
                     = pow(c, 0.8) * pow(upvoteRate * age, 0.8) / pow(age + 2, 1.8)

We then drop the constant pow(c, 0.8), and substitute in our Bayesian average estimate of the upvote rate, to get our final ranking formula:

    newRankingScore = pow((upvotes + weight) / (attention + weight) * age, 0.8) / pow(age + 2, 1.8)

## Discussion: Upvote Manipulation

Despite the [guideline](https://news.ycombinator.com/newsguidelines.html) not to solicit upvotes, people often do: for example when a Twitter user with a large following tells their followers to look for their post on the HN "new" page. [todo link].

This effect is largest for very new stories: even a handful of upvotes can result in a very high rank when the denominator (the age penalty) is still very small. This results of course in a feedback loop of ever higher rank and upvotes until the quadratic age penalty "catches up". 

Our proposed formula does not have such a feedback loop, because although stories at higher ranks accumulate upvotes more quickly, they also accumulate attention more quickly. So a story that "brings its own upvotes" will enjoy a higher rank for a moment, but it will not be able to sustain that rank unless upvotes increase in proportion to the increase in attention. In fact, the more successful an attacker is, the quicker the story will accumulate attention, and thus the sooner the score will approach the "true" upvote rate among HN users.

## Code

The application is a Go process running on a fly.io instance. The code is open source [github.com/social-protocols/news].

The application crawls the [Hacker News API] every minute. For each story, it records the current rank and page (top, new, best, etc.), and how many upvotes it has received. The HN API has an endpoint that returns the IDs of all the stories on each page in order, but getting the current number of upvotes for each story requires making a separate API call for each story. The application makes these requests in parallel so that this is fast and represents a point-in-time "snapshot". For each story, it calculates how many upvotes the attention for that time and rank and updates the accumulated attention for that story. The data is stored in a Sqlite database.

The frontpage generator queries the database and calculates the Bayesian average quality in the SQL query. It then uses the Go templating library to generate very simple HTML that mimics the original HN site. The frontpage is regenerated every minute and served compressed directly from memory.






