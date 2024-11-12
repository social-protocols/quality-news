package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	sqlite3 "github.com/mattn/go-sqlite3"

	stdlib "github.com/multiprocessio/go-sqlite3-stdlib"
	"github.com/pkg/errors"
	"golang.org/x/exp/slog"
)

type newsDatabase struct {
	*sql.DB

	db                            *sql.DB
	upvotesDB                     *sql.DB
	insertDataPointStatement      *sql.Stmt
	insertOrReplaceStoryStatement *sql.Stmt
	selectLastSeenScoreStatement  *sql.Stmt
	selectLastCrawlTimeStatement  *sql.Stmt
	selectStoryDetailsStatement   *sql.Stmt
	selectStoryCountStatement     *sql.Stmt

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

		`PRAGMA auto_vacuum=FULL`,
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

func (ndb newsDatabase) registerExtensions() error {
	conn, err := ndb.db.Conn(context.Background())
	if err != nil {
		return errors.Wrap(err, "getting connection")
	}
	defer conn.Close()

	err = conn.Raw(func(driverConn interface{}) error {
		if sqliteConn, ok := driverConn.(*sqlite3.SQLiteConn); ok {
			err := sqliteConn.RegisterFunc("sample_from_gamma_distribution", sampleFromGammaDistribution, true)
			if err != nil {
				return errors.Wrap(err, "sqliteConn.RegisterFunc(\"sample_from_gamma_distribution\")")
			}
		} else {
			return fmt.Errorf("failed to cast driverConn to *sqlite3.SQLiteConn")
		}
		return nil
	})
	return errors.Wrap(err, "registering sample_from_gamma_distribution")
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

	err = ndb.registerExtensions()
	if err != nil {
		return ndb, errors.Wrap(err, "ndb.registerExtensions()")
	}

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

	// the newsDatabase type has a few prepared statements that are defined here
	{
		sql := `
		INSERT INTO stories (id, by, title, url, timestamp, job) VALUES (?, ?, ?, ?, ?, ?) 
		ON CONFLICT DO UPDATE SET title = excluded.title, url = excluded.url, job = excluded.job
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
			, dupe
		) VALUES (
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?
		)
		`

		ndb.insertDataPointStatement, err = ndb.db.Prepare(sql) // Prepare statement.
		if err != nil {
			return ndb, errors.Wrap(err, "preparing insertDataPointStatement")
		}
	}

	{
		sql := `
		SELECT score, cumulativeUpvotes, cumulativeExpectedUpvotes, sampleTime
		FROM dataset
		WHERE id = ?
		ORDER BY sampleTime DESC LIMIT 1
		`
		ndb.selectLastSeenScoreStatement, err = ndb.db.Prepare(sql)
		if err != nil {
			return ndb, errors.Wrap(err, "preparing selectLastSeenScoreStatement")
		}
	}

	{
		sql := `
		SELECT ifnull(max(sampleTime),0) from dataset
		`
		ndb.selectLastCrawlTimeStatement, err = ndb.db.Prepare(sql)
		if err != nil {
			return ndb, errors.Wrap(err, "preparing selectLastCrawlTimeStatement")
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
			, unixepoch() - sampleTime + coalesce(ageApprox, sampleTime - submissionTime)
			, score
			, descendants
			, cumulativeUpvotes
			, cumulativeExpectedUpvotes
			-- , (cumulativeUpvotes + priorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + priorWeight) as quality 
			, penalty
			, topRank
			, qnRank
			, rawRank
			, flagged
			, dupe
			, job
			-- use latest information from the last available datapoint for this story (even if it is not in the latest crawl) *except* for rank information.
			from stories
			JOIN dataset
			USING (id)
			WHERE id = ?
			ORDER BY sampleTime DESC
			LIMIT 1
		`

		ndb.selectStoryDetailsStatement, err = ndb.db.Prepare(sql)
		if err != nil {
			return ndb, errors.Wrap(err, "preparing selectStoryDetailsStatement")
		}
	}

	{
		sql := `
		SELECT count(distinct id) from dataset
		`
		ndb.selectStoryCountStatement, err = ndb.db.Prepare(sql)
		if err != nil {
			return ndb, errors.Wrap(err, "preparing selectStoryCountStatement")
		}
	}

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
		d.dupe,
	)
	if err != nil {
		return err
	}
	return nil
}

func (ndb newsDatabase) insertOrReplaceStory(tx *sql.Tx, story Story) (int64, error) {
	stmt := tx.Stmt(ndb.insertOrReplaceStoryStatement)

	r, err := stmt.Exec(story.ID, story.By, story.Title, story.URL, story.SubmissionTime, story.Job)
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

	stmt := tx.Stmt(ndb.selectLastSeenScoreStatement)

	err := stmt.QueryRow(id).Scan(&score, &cumulativeUpvotes, &cumulativeExpectedUpvotes, &lastSeenTime)
	if err != nil {
		return score, cumulativeUpvotes, cumulativeExpectedUpvotes, lastSeenTime, err
	}

	return score, cumulativeUpvotes, cumulativeExpectedUpvotes, lastSeenTime, nil
}

func (ndb newsDatabase) selectLastCrawlTime() (int, error) {
	var sampleTime int

	err := ndb.selectLastCrawlTimeStatement.QueryRow().Scan(&sampleTime)

	return sampleTime, err
}

func (ndb newsDatabase) selectStoryDetails(id int) (Story, error) {
	var s Story

	err := ndb.selectStoryDetailsStatement.QueryRow(id).Scan(&s.ID, &s.By, &s.Title, &s.URL, &s.SubmissionTime, &s.OriginalSubmissionTime, &s.AgeApprox, &s.Score, &s.Comments, &s.CumulativeUpvotes, &s.CumulativeExpectedUpvotes, &s.Penalty, &s.TopRank, &s.QNRank, &s.RawRank, &s.Flagged, &s.Dupe, &s.Job)
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
