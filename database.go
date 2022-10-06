package main

import (
	"database/sql"
	"fmt"

	"github.com/johnwarden/hn"

	_ "github.com/mattn/go-sqlite3"

	stdlib "github.com/multiprocessio/go-sqlite3-stdlib"

	"os"

	"errors"
	"log"
)

type newsDatabase struct {
	db                            *sql.DB
	insertDataPointStatement      *sql.Stmt
	insertOrReplaceStoryStatement *sql.Stmt
	selectLastSeenScoreStatement  *sql.Stmt
}

func (ndb newsDatabase) close() {
	ndb.db.Close()
}

const sqliteDataFilename = "frontpage.sqlite"

func createDataDirIfNotExists(sqliteDataDir string) {

	if _, err := os.Stat(sqliteDataDir); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(sqliteDataDir, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func (ndb newsDatabase) init() {

	seedStatements := []string{
		"CREATE TABLE IF NOT EXISTS stories(id int primary key, by text not null, title text not null, url text not null, timestamp int not null);",
		"CREATE TABLE IF NOT EXISTS dataset (id integer not null, score integer, descendants integer not null, submissionTime integer not null, sampleTime integer not null, topRank integer, newRank integer, bestRank integer, askRank integer, showRank integer, cumulativeUpvotes real, cumulativeExpectedUpvotes real);",
		"CREATE INDEX IF NOT EXISTS dataset_sampletime_id ON dataset(sampletime, id);",
		"CREATE INDEX IF NOT EXISTS dataset_sampletime_id ON dataset(id);",
	}

	for _, s := range seedStatements {
		_, err := ndb.db.Exec(s)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func openNewsDatabase(sqliteDataDir string) (newsDatabase, error) {

	createDataDirIfNotExists(sqliteDataDir)

	frontpageDatabaseFilename := fmt.Sprintf("%s/%s", sqliteDataDir, sqliteDataFilename)

	ndb := newsDatabase{}

	var err error

	// Register some extension functions from go-sqlite3-stdlib so we can actually do math in sqlite3.
	stdlib.Register("sqlite3_ext")
	ndb.db, err = sql.Open("sqlite3_ext", frontpageDatabaseFilename)

	if err != nil {
		return ndb, err
	}

	ndb.init()

	{
		sql := `INSERT INTO stories (id, by, title, url, timestamp) VALUES (?, ?, ?, ?, ?) ON CONFLICT DO UPDATE SET title = excluded.title, url = excluded.url`
		ndb.insertOrReplaceStoryStatement, err = ndb.db.Prepare(sql)
		if err != nil {
			return ndb, err
		}
	}

	{
		sql := `INSERT INTO dataset (id, score, descendants, submissionTime, sampleTime, topRank, newRank, bestRank, askRank, showRank, cumulativeUpvotes, cumulativeExpectedUpvotes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		ndb.insertDataPointStatement, err = ndb.db.Prepare(sql) // Prepare statement.

		if err != nil {
			return ndb, err
		}
	}

	{
		sql := `SELECT score, cumulativeUpvotes, cumulativeExpectedUpvotes FROM dataset WHERE id = ? ORDER BY sampleTime DESC LIMIT 1`
		ndb.selectLastSeenScoreStatement, err = ndb.db.Prepare(sql)
		if err != nil {
			return ndb, err
		}
	}

	return ndb, nil

}

func (ndb newsDatabase) insertDataPoint(d dataPoint) error {
	_, err := ndb.insertDataPointStatement.Exec(d.id,
		d.score,
		d.descendants,
		d.submissionTime,
		d.sampleTime,
		rankToNullableInt(d.ranks[0]),
		rankToNullableInt(d.ranks[1]),
		rankToNullableInt(d.ranks[2]),
		rankToNullableInt(d.ranks[3]),
		rankToNullableInt(d.ranks[4]),
		d.cumulativeUpvotes,
		d.cumulativeExpectedUpvotes,
	)
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

func (ndb newsDatabase) selectLastSeenScore(id int) (int, int, float64, error) {
	var score int
	var cumulativeUpvotes int
	var cumulativeExpectedUpvotes float64
	err := ndb.selectLastSeenScoreStatement.QueryRow(id).Scan(&score, &cumulativeUpvotes, &cumulativeExpectedUpvotes)
	if err != nil {
		return score, cumulativeUpvotes, cumulativeExpectedUpvotes, err
	}
	return score, cumulativeUpvotes, cumulativeExpectedUpvotes, nil
}
