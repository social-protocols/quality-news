package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/johnwarden/hn"

	_ "github.com/mattn/go-sqlite3"

	stdlib "github.com/multiprocessio/go-sqlite3-stdlib"
)

type newsDatabase struct {
	db                            *sql.DB
	insertDataPointStatement      *sql.Stmt
	insertOrReplaceStoryStatement *sql.Stmt
	selectLastSeenScoreStatement  *sql.Stmt
	selectLastCrawlTimeStatement  *sql.Stmt
	selectStoryDetailsStatement   *sql.Stmt
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
		`
        CREATE TABLE IF NOT EXISTS stories(
            id int primary key
            , by text not null
            , title text not null
            , url text not null
            , timestamp int not null
        );
        `,
		`
        CREATE TABLE IF NOT EXISTS dataset (
            id integer not null
            , score integer
            , descendants integer not null
            , submissionTime integer not null
            , sampleTime integer not null
            , topRank integer
            , newRank integer
            , bestRank integer
            , askRank integer
            , showRank integer
            , qnRank integer
            , cumulativeUpvotes integer
            , cumulativeExpectedUpvotes real
            , qualityEstimate real
        );
        `,
		`
        CREATE INDEX IF NOT EXISTS dataset_sampletime_id
        ON dataset(sampletime, id);
        `,
		`
        CREATE INDEX IF NOT EXISTS dataset_id
        ON dataset(id);
        `,
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

	// the newsDatabase type has a few prepared statements that are defined here
	{
		sql := `
        INSERT INTO stories (id, by, title, url, timestamp) VALUES (?, ?, ?, ?, ?) 
        ON CONFLICT DO UPDATE SET title = excluded.title, url = excluded.url
        `
		ndb.insertOrReplaceStoryStatement, err = ndb.db.Prepare(sql)
		if err != nil {
			return ndb, err
		}
	}

	{
		sql := `
        INSERT INTO dataset (
            id
            , score
            , descendants
            , submissionTime
            , sampleTime
            , topRank
            , newRank
            , bestRank
            , askRank
            , showRank
            , cumulativeUpvotes
            , cumulativeExpectedUpvotes
        ) VALUES (
            ?, ?, ?, ?, ?,
            ?, ?, ?, ?, ?,
            ?, ?
        )
        `
		ndb.insertDataPointStatement, err = ndb.db.Prepare(sql) // Prepare statement.
		if err != nil {
			return ndb, err
		}
	}

	{
		sql := `
        SELECT score, cumulativeUpvotes, cumulativeExpectedUpvotes
        FROM dataset
        WHERE id = ?
        ORDER BY sampleTime DESC LIMIT 1
        `
		ndb.selectLastSeenScoreStatement, err = ndb.db.Prepare(sql)
		if err != nil {
			return ndb, err
		}
	}

	{
		sql := `
        SELECT ifnull(max(sampleTime),0) from dataset
        `
		ndb.selectLastCrawlTimeStatement, err = ndb.db.Prepare(sql)
		if err != nil {
			return ndb, err
		}
	}

	{
		sql := `
        SELECT 
            id
            , by
            , title
            , url
            , timestamp
            , score
            , descendants
            , (cumulativeUpvotes + ?)/(cumulativeExpectedUpvotes + ?) as quality 
            , topRank
            , qnRank
        FROM stories s
        JOIN dataset d
        USING (id)
        WHERE id = ?
        ORDER BY sampleTime DESC
        LIMIT 1
        `
		ndb.selectStoryDetailsStatement, err = ndb.db.Prepare(sql)
		if err != nil {
			return ndb, err
		}
	}

	return ndb, nil
}

func rankToNullableInt(rank int) (result sql.NullInt32) {
	if rank == 0 {
		result = sql.NullInt32{}
	} else {
		result = sql.NullInt32{Int32: int32(rank), Valid: true}
	}
	return
}

func (ndb newsDatabase) insertDataPoint(tx *sql.Tx, d dataPoint) error {
	stmt := tx.Stmt(ndb.insertDataPointStatement)

	_, err := stmt.Exec(d.id,
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

func (ndb newsDatabase) insertOrReplaceStory(tx *sql.Tx, story hn.Item) (int64, error) {
	if story.Type != "story" {
		return 0, nil
	}

	stmt := tx.Stmt(ndb.insertOrReplaceStoryStatement)

	r, err := stmt.Exec(story.ID, story.By, story.Title, story.URL, story.Timestamp)
	if err != nil {
		return 0, err
	}

	return r.RowsAffected()
}

func (ndb newsDatabase) selectLastSeenData(tx *sql.Tx, id int) (int, int, float64, error) {
	var score int
	var cumulativeUpvotes int
	var cumulativeExpectedUpvotes float64

	stmt := tx.Stmt(ndb.selectLastSeenScoreStatement)

	err := stmt.QueryRow(id).Scan(&score, &cumulativeUpvotes, &cumulativeExpectedUpvotes)
	if err != nil {
		return score, cumulativeUpvotes, cumulativeExpectedUpvotes, err
	}

	return score, cumulativeUpvotes, cumulativeExpectedUpvotes, nil
}

func (ndb newsDatabase) selectLastCrawlTime() (int, error) {
	var sampleTime int

	err := ndb.selectLastCrawlTimeStatement.QueryRow().Scan(&sampleTime)

	return sampleTime, err
}

func (ndb newsDatabase) selectStoryDetails(id int) (Story, error) {
	var s Story
	priorWeight := defaultFrontPageParams.PriorWeight

	err := ndb.selectStoryDetailsStatement.QueryRow(priorWeight, priorWeight, id).Scan(&s.ID, &s.By, &s.Title, &s.URL, &s.SubmissionTime, &s.Upvotes, &s.Comments, &s.Quality, &s.TopRank, &s.QNRank)
	if err != nil {
		return s, err
	}

	return s, nil
}
