package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"

	stdlib "github.com/multiprocessio/go-sqlite3-stdlib"
	"github.com/pkg/errors"
	"golang.org/x/exp/slog"
)

type newsDatabase struct {
	*sql.DB

	db                            *sql.DB
	insertDataPointStatement      *sql.Stmt
	insertOrReplaceStoryStatement *sql.Stmt
	selectLastSeenScoreStatement  *sql.Stmt
	selectLastCrawlTimeStatement  *sql.Stmt
	selectStoryDetailsStatement   *sql.Stmt
	selectStoryCountStatement     *sql.Stmt
}

func (ndb newsDatabase) close() {
	ndb.db.Close()
}

const sqliteDataFilename = "frontpage.sqlite"

func createDataDirIfNotExists(sqliteDataDir string) {
	if _, err := os.Stat(sqliteDataDir); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(sqliteDataDir, os.ModePerm)
		if err != nil {
			LogFatal(slog.Default(), "create sqlite data dir", err)
		}
	}
}

func (ndb newsDatabase) init() error {
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
			, score integer not null
			, descendants integer not null
			, sampleTime integer not null
			, submissionTime integer not null
			, topRank integer
			, newRank integer
			, bestRank integer
			, askRank integer
			, showRank integer
			, qnRank integer
			, cumulativeUpvotes integer
			, cumulativeExpectedUpvotes real
			, flagged boolean
			, job boolean
			, ageApprox int not null
			, penalty real not null default 0
			, currentPenalty real
		);
		`,
		`
		CREATE INDEX IF NOT EXISTS dataset_sampletime_id
		ON dataset(sampletime, id);
		`,
		`
	    CREATE INDEX IF NOT EXISTS dataset_id_sampletime
	    ON dataset(id, sampletime);
		`,
		`
		CREATE INDEX IF NOT EXISTS dataset_id
		ON dataset(id);
		`,
	}

	for _, s := range seedStatements {
		_, err := ndb.db.Exec(s)
		if err != nil {
			return errors.Wrap(err, "seeding database")
		}
	}
	return nil
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

	err = ndb.init()
	if err != nil {
		return ndb, err
	}

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
			, sampleTime
			, submissionTime
			, ageApprox
			, topRank
			, newRank
			, bestRank
			, askRank
			, showRank
			, cumulativeUpvotes
			, cumulativeExpectedUpvotes
			, flagged
			, job
		) VALUES (
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?
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
			, submissionTime
			, timestamp as originalSubmissionTime
			, ageApprox
			, score-1
			, descendants
			, (cumulativeUpvotes + ?)/(cumulativeExpectedUpvotes + ?) as quality 
			, penalty
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

	{
		sql := `
		SELECT count(distinct id) from dataset
		`
		ndb.selectStoryCountStatement, err = ndb.db.Prepare(sql)
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
		d.sampleTime,
		d.submissionTime,
		d.ageApprox,
		rankToNullableInt(d.ranks[0]),
		rankToNullableInt(d.ranks[1]),
		rankToNullableInt(d.ranks[2]),
		rankToNullableInt(d.ranks[3]),
		rankToNullableInt(d.ranks[4]),
		d.cumulativeUpvotes,
		d.cumulativeExpectedUpvotes,
		d.flagged,
		d.job,
	)
	if err != nil {
		return err
	}
	return nil
}

func (ndb newsDatabase) insertOrReplaceStory(tx *sql.Tx, story Story) (int64, error) {
	stmt := tx.Stmt(ndb.insertOrReplaceStoryStatement)

	r, err := stmt.Exec(story.ID, story.By, story.Title, story.URL, story.SubmissionTime)
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

	err := ndb.selectStoryDetailsStatement.QueryRow(priorWeight, priorWeight, id).Scan(&s.ID, &s.By, &s.Title, &s.URL, &s.SubmissionTime, &s.OriginalSubmissionTime, &s.AgeApprox, &s.Score, &s.Comments, &s.Quality, &s.Penalty, &s.TopRank, &s.QNRank)
	if err != nil {
		return s, err
	}

	return s, nil
}

func (ndb newsDatabase) storyCount(tx *sql.Tx) (int, error) {
	var count int

	stmt := tx.Stmt(ndb.selectStoryCountStatement)

	err := stmt.QueryRow().Scan(&count)

	return count, err
}
