Quality News is a Hacker News client that provides additional charts and data about the performance of stories. 

<!--
To Generate This HTML:

pandoc --from gfm --to html about.md > templates/about-content.html.tmpl
-->

## Upvote Rate

The **upvote rate** quantifies how much more or less likely users are to upvote this story compared to the average story. It is calculated as total upvotes divided by total **expected upvotes**.

## Expected Upvotes

The **expected upvotes** for a story is an estimate of the number of upvotes the **average story** would have received if it were shown at the same times at the same ranks.

## Rank Difference

The difference between the story's actual rank and it's **raw rank**, which is the rank that a story would have according to the "raw" Hacker News ranking formula:

	upvotes^0.8 / (ageHours+2)^1.8

The actual Hacker News ranking formula applies additional adjustments to ranking score based on a variety of factors: penalties for flagged stories, bonuses for stories in the 2nd-chance queue, and other factors which are not entirely known. 

## Second-Chance Age

The story's revised age after being re-posted from the the [2nd-chance queue](https://news.ycombinator.com/item?id=19774614).

## More Details

See the <a href="https://github.com/social-protocols/news#readme">Readme</a> on Github for more details.
