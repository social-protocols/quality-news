package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/exp/slog"
)

const (
	qnRankFormulaSQL = "pow(ageHours * (cumulativeUpvotes + overallPriorWeight)/((1-exp(-fatigueFactor*cumulativeExpectedUpvotes))/fatigueFactor + overallPriorWeight), 0.8) / pow(ageHours + 2, gravity/0.8) desc"

	// qnRankFormulaSQL = `
	// 	pow(
	// 		ageHours *
	// 		sample_from_gamma_distribution(
	// 			cumulativeUpvotes + overallPriorWeight,
	// 			(
	// 					1-exp(-fatigueFactor*cumulativeExpectedUpvotes)
	// 			) / fatigueFactor + overallPriorWeight
	// 		 )
	// 		 , 0.8
	// 	) / pow(
	// 			ageHours + 2
	// 			, gravity/0.8
	// 	) desc`

	hnRankFormulaSQL = "(score-1) / pow(ageHours + 2, gravity/0.8) desc"
)

func (app app) crawlPostprocess(ctx context.Context, tx *sql.Tx) error {
	t := time.Now()
	defer crawlPostprocessingDuration.UpdateDuration(t)

	var err error

	// for _, filename := range []string{"previous-crawl.sql", "resubmissions.sql", "raw-ranks.sql", "upvote-rates.sql"} {
	for _, filename := range []string{
		"previous-crawl.sql",
		"resubmissions.sql",
		"raw-ranks.sql",
		"latest-story-stats.sql",
	} {
		app.logger.Info("Processing SQL file", slog.String("filename", filename))
		err = executeSQLFile(ctx, tx, filename)
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
	sql := fmt.Sprintf(qnRanksSQL, d.PriorWeight, d.OverallPriorWeight, d.Gravity, d.PenaltyWeight, d.FatigueFactor, qnRankFormulaSQL)

	stmt, err := tx.Prepare(sql)
	if err != nil {
		return errors.Wrap(err, "preparing updateQNRanksSQL")
	}

	_, err = stmt.ExecContext(ctx)

	app.logger.Info("Finished executing updateQNRanks", slog.Duration("elapsed", time.Since(t)))

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

func executeSQLFile(ctx context.Context, tx *sql.Tx, filename string) error {
	sql := readSQLSource(filename)

	sql = strings.Trim(sql, " \n\r;")

	parts := strings.Split(sql, ";\n")

	for _, sql := range parts {

		stmt, err := tx.Prepare(sql)
		if err != nil {
			return errors.Wrapf(err, "preparing SQL in file %s", filename)
		}

		_, err = stmt.ExecContext(ctx)

		if err != nil {
			return errors.Wrapf(err, "executing SQL in file %s", filename)
		}
	}
	return nil
}
