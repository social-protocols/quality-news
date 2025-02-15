package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	stdlib "github.com/multiprocessio/go-sqlite3-stdlib"
	"github.com/pkg/errors"
	"golang.org/x/exp/slog"
)

type newsDatabase struct {
	*sql.DB

	db            *sql.DB
	upvotesDB     *sql.DB
	sqliteDataDir string
}

/* Attach the frontpage dataset for each context, to solve "no such table" errors,

per suggestion here https://stackoverflow.com/users/saves/2573589
*/

func (ndb newsDatabase) upvotesDBWithDataset(ctx context.Context) (*sql.Conn, error) {
	conn, err := ndb.upvotesDB.Conn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "ndb.upvotesDB.Conn")
	}

	frontpageDatabaseFilename := fmt.Sprintf("%s/%s", ndb.sqliteDataDir, sqliteDataFilename)

	// attach frontpage database as readonly. This way, we can write to the upvotes database while the crawler
	// is writing to the frontpage database.
	s := fmt.Sprintf("attach database 'file:%s?mode=ro' as frontpage", frontpageDatabaseFilename)
	_, err = conn.ExecContext(ctx, s)
	if err != nil && err.Error() != "database frontpage is already in use" {
		return conn, errors.Wrap(err, "attach frontpage database")
	}

	return conn, nil
}

func (ndb newsDatabase) attachFrontpageDB() error {
	frontpageDatabaseFilename := fmt.Sprintf("%s/%s", ndb.sqliteDataDir, sqliteDataFilename)

	s := fmt.Sprintf("attach database 'file:%s?mode=ro' as frontpage", frontpageDatabaseFilename)

	_, err := ndb.upvotesDB.Exec(s)

	if err != nil && err.Error() != "database frontpage is already in use" {
		return errors.Wrap(err, "attach frontpage database")
	}

	return nil
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

func (ndb newsDatabase) initFrontpageDB() error {
	seedStatements := []string{
		`
		CREATE TABLE IF NOT EXISTS stories(
			id int primary key
			, by text not null
			, title text not null
			, url text not null
			, timestamp int not null
			, job boolean not null default false
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
			, cumulativeUpvotes integer not null default 0
			, cumulativeExpectedUpvotes real not null default 0
			, flagged boolean not null default false
			, dupe boolean not null default false
			, ageApprox int not null
			, penalty real not null default 0
			, currentPenalty real
			, rawRank int
			, upvoteRate float not null default 1
			, upvoteRateWindow int
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
		`
		drop view if exists previousCrawl
		`,
		`PRAGMA auto_vacuum=NONE`,
	}

	for _, s := range seedStatements {
		_, err := ndb.db.Exec(s)
		if err != nil {
			return errors.Wrapf(err, "seeding database: %s", s)
		}
	}

	alterStatements := []string{
		`alter table dataset add column upvoteRateWindow int`,
		`alter table dataset add column upvoteRate float default 0 not null`,
		`alter table stories add column archived boolean default false not null`,
		`DROP INDEX if exists archived`,
		`CREATE INDEX IF NOT EXISTS dataset_sampletime on dataset(sampletime)`,

		`update dataset set upvoteRate = ( cumulativeUpvotes + 2.3 ) / ( cumulativeExpectedUpvotes + 2.3) where upvoteRate = 0`,
	}

	for _, s := range alterStatements {
		_, _ = ndb.db.Exec(s)
	}

	return nil
}

func (ndb newsDatabase) initUpvotesDB() error {
	seedStatements := []string{
		`create table if not exists votes(userID int not null, storyID int not null, direction int8 not null, entryTime int not null, entryUpvotes int not null, entryExpectedUpvotes int not null)`,
		`create index if not exists votes_ids on votes(storyID, userID)`,
		`create index if not exists votes_storyID on votes(storyID)`,
		`create index if not exists votes_userid on votes(userID)`,
		`drop view if exists positions`,
		`create view if not exists positions as
			with exits as ( 
			select
			  votes.rowID as positionID
			  , votes.*
			  , first_value(entryTime) over ( partition by userID, storyID order by entryTime rows between current row and unbounded following exclude current row) as exitTime
			  , first_value(entryUpvotes) over ( partition by userID, storyID order by entryTime rows between current row and unbounded following exclude current row) as exitUpvotes
			  , first_value(entryExpectedUpvotes) over ( partition by userID, storyID order by entryTime rows between current row and unbounded following exclude current row) as exitExpectedUpvotes
			from votes
			) select * from exits where direction != 0
		`,
	}

	for _, s := range seedStatements {
		_, err := ndb.upvotesDB.Exec(s)
		if err != nil {
			return errors.Wrapf(err, "seeding votes database: %s", s)
		}
	}

	alterStatements := []string{}

	for _, s := range alterStatements {
		_, _ = ndb.upvotesDB.Exec(s)
	}

	frontpageDatabaseFilename := fmt.Sprintf("%s/%s", ndb.sqliteDataDir, sqliteDataFilename)

	// attach the dataset table
	s := fmt.Sprintf("attach database 'file:%s?mode=ro' as frontpage", frontpageDatabaseFilename)
	_, err := ndb.upvotesDB.Exec(s)
	return errors.Wrap(err, "attach frontpage database")
}

func openNewsDatabase(sqliteDataDir string) (newsDatabase, error) {
	createDataDirIfNotExists(sqliteDataDir)

	frontpageDatabaseFilename := fmt.Sprintf("%s/%s", sqliteDataDir, sqliteDataFilename)

	ndb := newsDatabase{sqliteDataDir: sqliteDataDir}

	var err error

	// Register some extension functions from go-sqlite3-stdlib so we can actually do math in sqlite3.
	stdlib.Register("sqlite3_ext")

	// Connect to database
	ndb.db, err = sql.Open("sqlite3_ext", fmt.Sprintf("file:%s?_journal_mode=WAL", frontpageDatabaseFilename))

	if err != nil {
		return ndb, errors.Wrap(err, "open frontpageDatabase")
	}

	// err = ndb.registerExtensions()
	// if err != nil {
	// 	return ndb, errors.Wrap(err, "ndb.registerExtensions()")
	// }

	err = ndb.initFrontpageDB()
	if err != nil {
		return ndb, errors.Wrap(err, "init frontpageDatabase")
	}

	{
		upvotesDatabaseFilename := fmt.Sprintf("%s/upvotes.sqlite", sqliteDataDir)

		ndb.upvotesDB, err = sql.Open("sqlite3_ext", fmt.Sprintf("file:%s?_journal_mode=WAL", upvotesDatabaseFilename))
		if err != nil {
			return ndb, errors.Wrap(err, "open upvotesDB")
		}

		err = ndb.initUpvotesDB()
		if err != nil {
			return ndb, errors.Wrap(err, "initUpvotesDB")
		}

	}

	// No need to prepare statements anymore as we're removing prepared statements

	err = ndb.importPenaltiesData(sqliteDataDir)

	return ndb, err
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
	sqlStatement := `
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
			, dupe
		) VALUES (
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?
		)
	`

	_, err := tx.Exec(sqlStatement,
		d.id,
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
		d.dupe,
	)
	if err != nil {
		return err
	}
	return nil
}

func (ndb newsDatabase) insertOrReplaceStory(tx *sql.Tx, story Story) (int64, error) {
	sqlStatement := `
		INSERT INTO stories (id, by, title, url, timestamp, job) VALUES (?, ?, ?, ?, ?, ?) 
		ON CONFLICT DO UPDATE SET title = excluded.title, url = excluded.url, job = excluded.job
	`

	r, err := tx.Exec(sqlStatement, story.ID, story.By, story.Title, story.URL, story.SubmissionTime, story.Job)
	if err != nil {
		return 0, err
	}

	return r.RowsAffected()
}

func (ndb newsDatabase) selectLastSeenData(tx *sql.Tx, id int) (int, int, float64, int, error) {
	var score int
	var cumulativeUpvotes int
	var cumulativeExpectedUpvotes float64
	var lastSeenTime int

	sqlStatement := `
		SELECT score, cumulativeUpvotes, cumulativeExpectedUpvotes, sampleTime
		FROM dataset
		WHERE id = ?
		ORDER BY sampleTime DESC LIMIT 1
	`

	err := tx.QueryRow(sqlStatement, id).Scan(&score, &cumulativeUpvotes, &cumulativeExpectedUpvotes, &lastSeenTime)
	if err != nil {
		return score, cumulativeUpvotes, cumulativeExpectedUpvotes, lastSeenTime, err
	}

	return score, cumulativeUpvotes, cumulativeExpectedUpvotes, lastSeenTime, nil
}

func (ndb newsDatabase) selectLastCrawlTime() (int, error) {
	var sampleTime int

	sqlStatement := `
		SELECT ifnull(max(sampleTime),0) from dataset
	`

	err := ndb.db.QueryRow(sqlStatement).Scan(&sampleTime)

	return sampleTime, err
}

func (ndb newsDatabase) selectStoriesToArchive(ctx context.Context) ([]int, error) {
	var storyIDs []int

	sqlStatement := `
			with latest as (
				select id, score, sampleTime from dataset
				where sampleTime <= unixepoch() - 21*24*60*60
				and score > 2
				order by sampleTime
				limit 100
			)
			select distinct id from latest limit 10
	`

	// Check context before query
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context cancelled before query")
	}

	rows, err := ndb.db.QueryContext(ctx, sqlStatement)
	if err != nil {
		if err == context.DeadlineExceeded || err == context.Canceled {
			return nil, errors.Wrap(err, "context cancelled during query")
		}
		return storyIDs, errors.Wrap(err, "selectStoriesToArchive QueryContext")
	}
	defer rows.Close()

	for rows.Next() {
		// Check context in loop
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "context cancelled during row iteration")
		}

		var storyID int
		if err := rows.Scan(&storyID); err != nil {
			return nil, errors.Wrap(err, "scan story ID")
		}
		storyIDs = append(storyIDs, storyID)
	}
	if err := rows.Err(); err != nil {
		if err == context.DeadlineExceeded || err == context.Canceled {
			return nil, errors.Wrap(err, "context cancelled after row iteration")
		}
		return nil, errors.Wrap(err, "iterating story IDs")
	}
	return storyIDs, nil
}

func (ndb newsDatabase) purgeStory(ctx context.Context, storyID int) error {
	tx, err := ndb.db.Begin()
	if err != nil {
		return errors.Wrap(err, "starting transaction")
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		} else if err != nil {
			_ = tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	// Delete all data points
	_, err = tx.ExecContext(ctx, `DELETE FROM dataset WHERE id = ?`, storyID)
	if err != nil {
		return errors.Wrap(err, "delete from dataset")
	}

	// Delete story record
	_, err = tx.ExecContext(ctx, `DELETE FROM stories WHERE id = ?`, storyID)
	if err != nil {
		return errors.Wrap(err, "delete from stories")
	}

	return nil
}

func (ndb newsDatabase) selectStoryDetails(id int) (Story, error) {
	var s Story

	sqlStatement := `
	SELECT
		id
		, by
		, title
		, url
		, submissionTime
		, timestamp as originalSubmissionTime
		, unixepoch() - sampleTime + coalesce(ageApprox, sampleTime - submissionTime)
		, score
		, descendants
		, cumulativeUpvotes
		, cumulativeExpectedUpvotes
		, topRank
		, qnRank
		, rawRank
		, flagged
		, dupe
		, job
		, archived
	from stories
	JOIN dataset
	USING (id)
	WHERE id = ?
	ORDER BY sampleTime DESC
	LIMIT 1
	`

	err := ndb.db.QueryRow(sqlStatement, id).Scan(&s.ID, &s.By, &s.Title, &s.URL, &s.SubmissionTime, &s.OriginalSubmissionTime, &s.AgeApprox, &s.Score, &s.Comments, &s.CumulativeUpvotes, &s.CumulativeExpectedUpvotes, &s.TopRank, &s.QNRank, &s.RawRank, &s.Flagged, &s.Dupe, &s.Job, &s.Archived)
	if err != nil {
		return s, err
	}

	return s, nil
}

func (ndb *newsDatabase) resetConnection() error {
	// Create new connections first
	frontpageDatabaseFilename := fmt.Sprintf("%s/%s", ndb.sqliteDataDir, sqliteDataFilename)
	newDB, err := sql.Open("sqlite3_ext", fmt.Sprintf("file:%s?_journal_mode=WAL", frontpageDatabaseFilename))
	if err != nil {
		return errors.Wrap(err, "reopen frontpageDatabase")
	}

	upvotesDatabaseFilename := fmt.Sprintf("%s/upvotes.sqlite", ndb.sqliteDataDir)
	newUpvotesDB, err := sql.Open("sqlite3_ext", fmt.Sprintf("file:%s?_journal_mode=WAL", upvotesDatabaseFilename))
	if err != nil {
		newDB.Close()
		return errors.Wrap(err, "reopen upvotesDB")
	}

	// Swap to new connections
	ndb.db = newDB
	ndb.upvotesDB = newUpvotesDB

	return nil
}

func (ndb newsDatabase) storyCount(tx *sql.Tx) (int, error) {
	var count int

	sqlStatement := `
		SELECT count(distinct id) from stories
	`

	row := tx.QueryRow(sqlStatement)
	if row == nil {
		return 0, errors.New("invalid transaction state in storyCount")
	}

	err := row.Scan(&count)
	if err != nil {
		// We can't reset connection during transaction, so just return the error
		return 0, errors.Wrap(err, "scanning story count")
	}

	return count, nil
}

func (ndb newsDatabase) vacuumIfNeeded(ctx context.Context, logger *slog.Logger) error {
	size, freelist, fragmentation, err := ndb.getDatabaseStats()
	if err != nil {
		return errors.Wrap(err, "getDatabaseStats")
	}

	logger.Info("Database stats",
		"size_mb", float64(size)/(1024*1024),
		"freelist_pages", freelist,
		"fragmentation_pct", fragmentation)

	if fragmentation > 20.0 {
		logger.Info("Starting vacuum operation",
			"fragmentation_pct", fragmentation)

		startTime := time.Now()
		_, err := ndb.db.ExecContext(ctx, "VACUUM")
		if err != nil {
			return errors.Wrap(err, "vacuum database")
		}

		newSize, _, newFragmentation, err := ndb.getDatabaseStats()
		if err != nil {
			return errors.Wrap(err, "getDatabaseStats after vacuum")
		}

		logger.Info("Vacuum completed",
			"duration_seconds", time.Since(startTime).Seconds(),
			"size_before_mb", float64(size)/(1024*1024),
			"size_after_mb", float64(newSize)/(1024*1024),
			"space_reclaimed_mb", float64(size-newSize)/(1024*1024),
			"fragmentation_before", fragmentation,
			"fragmentation_after", newFragmentation)

		vacuumOperationsTotal.Inc()
	}

	return nil
}

func (ndb newsDatabase) getDatabaseStats() (size int64, freelist int64, fragmentation float64, err error) {
	err = ndb.db.QueryRow(`
		SELECT 
			(SELECT page_count FROM pragma_page_count()) * 
			(SELECT page_size FROM pragma_page_size()) as total_bytes,
			(SELECT freelist_count FROM pragma_freelist_count()) as free_pages,
			ROUND(
				100.0 * (SELECT freelist_count FROM pragma_freelist_count()) /
				(SELECT page_count FROM pragma_page_count()), 1
			) as fragmentation_pct
	`).Scan(&size, &freelist, &fragmentation)
	return
}

func (ndb newsDatabase) deleteOldData(ctx context.Context) (int64, error) {
	// Delete data older than one month. It's taking about 18 days to fill a gig, and we have
	// 3 gig volume currently. That gives us 54 days. But 30 days is plenty and this gives us margin
	// in case the rate of growth of data increases.
	sqlStatement := `
		delete from dataset where id in
		(select distinct id from dataset join stories using (id) where timestamp <= unixepoch()-30*24*60*60)
	`

	result, err := ndb.db.ExecContext(ctx, sqlStatement)
	if err != nil {
		return 0, errors.Wrap(err, "executing delete old data query")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, "getting rows affected")
	}

	return rowsAffected, nil
}
