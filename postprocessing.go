package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/exp/slog"
)

const qnRankFormulaSQL = "pow(ageHours, (cumulativeUpvotes + overallPriorWeight)/(cumulativeExpectedUpvotes + overallPriorWeight)) / pow(ageHours + 2, gravity/0.8) desc"

func (app app) crawlPostprocess(ctx context.Context, tx *sql.Tx) error {
	t := time.Now()
	defer crawlPostprocessingDuration.UpdateDuration(t)

	var err error

	for _, filename := range []string{"previous-crawl.sql", "resubmissions.sql", "expected-ranks.sql"} {
		err = app.ndb.executeSQLFile(ctx, tx, filename)
		if err != nil {
			return err
		}
	}

	err = app.updateQNRanks(ctx, tx)
	if err != nil {
		return errors.Wrap(err, "updateQNRanks")
	}

	app.logger.Info("Finished crawl postprocessing", slog.Duration("elapsed", time.Since(t)))

	return err
}

var qnRanksSQL = readSQLSource("qnranks.sql")

func (app app) updateQNRanks(ctx context.Context, tx *sql.Tx) error {
	t := time.Now()

	d := defaultFrontPageParams
	sql := fmt.Sprintf(qnRanksSQL, d.PriorWeight, d.OverallPriorWeight, d.Gravity, d.PenaltyWeight, qnRankFormulaSQL)

	stmt, err := tx.Prepare(sql)
	if err != nil {
		return errors.Wrap(err, "preparing updateQNRanksSQL")
	}

	_, err = stmt.ExecContext(ctx)

	app.logger.Debug("Finished executing updateQNRanks", slog.Duration("elapsed", time.Since(t)))

	return errors.Wrap(err, "executing updateQNRanksSQL")
}

func readSQLSource(filename string) string {
	f, err := resources.Open("sql/" + filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	buf := bytes.NewBuffer(nil)
	_, err = io.Copy(buf, f)
	if err != nil {
		panic(err)
	}

	return buf.String()
}

func (ndb newsDatabase) executeSQLFile(ctx context.Context, tx *sql.Tx, filename string) error {
	sql := readSQLSource(filename)

	stmt, err := tx.Prepare(sql)
	if err != nil {
		return errors.Wrapf(err, "preparing SQL in file %s", filename)
	}

	_, err = stmt.ExecContext(ctx)

	return errors.Wrapf(err, "executing SQL in file %s", filename)
}
