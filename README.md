To Run Locally:

	Set SQLITE_DATA_DIR to the location of the directory containing hacker-news.sqlite (created by https://github.com/social-protocols)
	
	SQLITE_DATA_DIR=/data-directory go run app.go

To deploy to fly.io:

	// TODO: Copy the sqlite data file to the fly.io data volume

	`flyctl launch`

View the deployed app with:

	`flyctl open`
