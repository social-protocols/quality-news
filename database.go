package main

import (
	"database/sql"
	"fmt"

	"github.com/johnwarden/hn"

	_ "github.com/mattn/go-sqlite3"
)

type newsDatabase struct {
	db                            *sql.DB
	insertDataPointStatement      *sql.Stmt
	insertOrReplaceStoryStatement *sql.Stmt
}

func (ndb newsDatabase ) close() {
	ndb.db.Close()
}

func openNewsDatabase(sqliteDataDir string) (newsDatabase, error) {

	frontpageDatabaseFilename := fmt.Sprintf("%s/frontpage.sqlite", sqliteDataDir)

	ndb := newsDatabase{}

	var err error

	ndb.db, err = sql.Open("sqlite3", frontpageDatabaseFilename)

	if err != nil {
		return ndb, err
	}

	{
		sql := `INSERT OR REPLACE INTO stories (id, by, title, url, timestamp) VALUES (?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`
		ndb.insertOrReplaceStoryStatement, err = ndb.db.Prepare(sql)
		if err != nil {
			return ndb, err
		}
	}

	{
		sql := `INSERT INTO dataset (id, score, descendants, submissionTime, sampleTime, topRank, newRank, bestRank, askRank, showRank) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		ndb.insertDataPointStatement, err = ndb.db.Prepare(sql) // Prepare statement.

		if err != nil {
			return ndb, err
		}
	}

	return ndb, nil

}

func (ndb newsDatabase) insertDataPoint(d dataPoint) error {
	_, err := ndb.insertDataPointStatement.Exec(d.id, d.score, d.descendants, d.submissionTime, d.sampleTime, rankToNullableInt(d.ranks[0]), rankToNullableInt(d.ranks[1]), rankToNullableInt(d.ranks[2]), rankToNullableInt(d.ranks[3]), rankToNullableInt(d.ranks[4]))
	if err != nil {
		return err
	}
	return nil
}

func (ndb newsDatabase) insertOrReplaceStory(story hn.Item) error {

	if story.Type != "story" {
		return nil
	}

	_, err := ndb.insertOrReplaceStoryStatement.Exec(story.ID, story.By, story.Title, story.URL, story.Timestamp)
	if err != nil {
		return err
	}
	return nil

}
