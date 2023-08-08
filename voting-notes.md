# Voting Notes

## TODO

Disable upvote buttons for jobs

## Login/Logout

I have created simple login/logout functionality:
	
	Login with random user ID: 	
		/login
	Login with specific user user ID: 
		/login?userID=1234
	Logout user: 
		/logout

If you are logged-in, your user-id will be shown on the top right, and upvote/downvote buttons will be shown next to stories. 

You can toggle a vote to clear the vote. Switching from upvote to downvote or vice versa first clears the current vote.

## Votes and Positions Tables

The `votes` table has one entry for each change of position (from upvoted to cleared, cleared to upvoted, downvoted to upvoted, etc.)

The `positions` view is like the `votes` table, but it does not contain a record for when a vote is cleared. Instead, it contains one record for each upvote/downvote, along with score/price details for the moment the upvote/downvote happened, and then the moment that the position was exited, (the moment the the vote was cleared, if any).

## Scoring

Voting is like buying a stock. Your score is based on your entry price and your final price, which is either the exit price (if you exited the position), or the current price. 

If the final price is greater then the entry price, you gain points, if it is less, you lose. There are a couple of different scoring formulas. 

## Score Page

The score page is at:

	/score

The score page shows each "position". Since a user can enter/exit a position on a story multiple times, a story might be shown multiple times. The users total score is the sum of the score for all positions.

You can look at the score for a particular user:

		/score?userID=1234

You can also use different scoring formulas

		/score?scoringFormula=InformationGain
		/score?scoringFormula=PTS     	# Peer Truth-Serum
		/score?scoringFormula=LogPTS    # Default Formula: Log Peer Truth-Serum

And change the model parameters, the most important of which is the priorWeight

		/score?priorWeight = 3.5

## Baseline User IDs

UserID 0 randomly votes on stories on the new page.

		/score?userID=0

UserID 1 randomly votes on stories on the front page.

		/score?userID=1

Upvotes all new stories immediately (on first crawl where they appear)

		/score?userID=2

Downvotes all new stories immediately

		/score?userID=3


## IMPORTANT FINDINGS

- We need to constantly tune priorWeight so that the results of random voting average to 0.
	We get different results for the total score for userID 0 than we get from compare-against-random-voter. This is because userID 0 has a starting price that is generally slightly smaller than the priorAverage, because for some stories we accumulate some attention in the first data point. So userID 0 waits for the first data point and then votes, thereby getting in at a slightly lower price. We want this not to be a viable strategy, and it seems we can do this if we just tune down the priorWeight.

- The scoring formula seems to work best if the user's vote is not counted in upvote rate calculations -- either in the final upvote rate, or in the entry upvote rate. This means if users vote through our platform, we need to count the entry upvote rate **before** their vote. Then, we need to subtract their vote out when calculating final upvote rate.
	- Why is this? One, intuitively, the scoring formula seems to give me more reasonable (e.g. higher) scores this way
	- Two, it is closer to PTS, where neither the numerator nor the denominator factor in the user's vote.


# Information-Theory Scoring Formula

Okay, let's say the user provides information that increases the upvote rate from R to S.

views = W = A*n
upvotes = U = A*n*p
upvoteRate = R = U/A and thus 
R = np


The total surprise is expected value of surprise times the number of views

	A*n * (p * log(p) + (1 - p) * log(1-p))

If p is the posterior probability (before user's vote), and q is the prior probability, then the surprise from the fully-informed point of view (whatever we call it), that is the expected value (over p) of the surprise of q, is 

	A*n * (p * log(q) + (1 - p) * log(1-q))

And the difference is 

	A*n * (p * log(p) + (1 - p) * log(1-p))
	- A*n * (p * log(q) + (1 - p) * log(1-q))
	= A*n * p*log(p/q) + (1-p)log((1-p)/(1-q))


Which is the KL divergence times number of views

A*n * Dkl(p||q) 
	= A*n * (p * log(p/q) + (1 - p) * log((1-p)/(q-q)))

Now given 

	R = pn
	S = qn
	p = R/n
	q = S/n

We can rewrite that as

	= A ( n*p*log(p/q) + n(1 - p)log((1-p)/(1-q)) )
	= A ( Rlog(R/n / S/n) + n*log((1-R/n)/(1-S/n)) - R*log((1-R/n)/(1-S/n)) )
	= A ( Rlog(R/S) + n*log((1-R/n)/(1-S/n)) - R*log((n-R)/(n-S)) )
	= A ( Rlog(R/S) + n*log(1-R/n) - n*log(1-S/n) - R*log((n-R)/(n-S)) )

Now we want to find the limit of this as n approaches infinity.

	(n-R)/(n-S) approaches 1, therefore log( (n-R)/(n-S) ) approaches 0

So now we want is

	lim{n->∞} A ( Rlog(R/S) + n*log(1-R/n) - n*log(1-S/n) )

As n approaches infinity

Now here is a key insight!

lim{n->∞} n * ln(1 - c/n) = -c

So converting our formula to use natural logarithm

	lim{n->∞}  ( R×ln(R/S) + n×ln(1 − R/n) − n×ln(1 − S/n) ) * A / ln(2)

And using substituting lim{n->∞} n×ln(1 − S/n) = -S and lim{n->∞} n×ln(1 − R/n) = -R

	lim{n->∞}  ( R×ln(R/S) -R + S ) * A / ln(2)

	= ( R×ln(R/S) - R + S ) * A / ln(2)


	= ( R(ln(R/S) - 1) + S ) * A / ln(2)

	if R = U/A and S = V/A

	= ( U(ln(R/S) - 1) + V ) / ln(2)


	= A * (R*log(R/S) + (S-R)/ln(2) )

Okay now the idea is that this is the **total** information value of all upvotes. Each individual upvotes incrementally changes the estimated upvote from Rj to Rk. The final upvote rate is R, the final probability is p. The final views are An. That information gain is mutiplied by all **subsequent** views, which is (A - An)n. 
	if j = k-1

	(A - Ak)n * (p * log(pk) + (1 - p) * log(1-pk))
	- (A - Ak)n * (p * log(pj) + (1 - p) * log(1-pj))
	= (A - Ak)n*n * p*log(pk/pj) + (1-p)log((1-pk)/(1-pj))

	= (A - Ak) ( Rlog(Rk/Rj) + n*log(1-Rk/n) - n*log(1-Rj/n) - R*log((n-Rk)/(n-Rj)) )

	lim{n->∞} of that is

	= (A - Ak) ( Rlog(Rk/Rj) + n*log(1-Rk/n) - n*log(1-Rj/n)  )
	= (A - Ak) ( Rln(Rk/Rj) + n*ln(1-Rk/n) - n*ln(1-Rj/n) ) / ln(2)

	= (A - Ak) ( Rlog(Rk/Rj) + (Rj - Rk)/ln2 )

Or we can go with the KL divergence between two poisson distributions, which is:

https://stats.stackexchange.com/questions/145789/kl-divergence-between-two-univariate-poisson-distributions
	
𝐷ₖₗ(𝑓₁||𝑓₂)=𝜆₁log(𝜆₁𝜆₂)+𝜆₂−𝜆₁

----


Okay but how do we convert this to value created? 


Do we credit users for more value creation for upvoting stories that ultimately get a lot of upvotes? I think not actually. The value created on the home page is a result of all the information provided. 

So I think we should look at total value created during some period of time, and give credit to users proportionally to the amount of information they provided for that period of time.


