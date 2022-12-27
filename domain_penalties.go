package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/pkg/errors"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DomainPenalty struct {
	Domain     string `gorm:"primaryKey"`
	AvgPenalty float64
}

func (ndb newsDatabase) importPenaltiesData(sqliteDataDir string) error {
	frontpageDatabaseFilename := fmt.Sprintf("%s/%s", sqliteDataDir, sqliteDataFilename)

	db, err := gorm.Open(sqlite.Open(frontpageDatabaseFilename), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	err = db.AutoMigrate(&DomainPenalty{})
	if err != nil {
		return errors.Wrap(err, "db.AutoMigrate Domain Penalties table")
	}

	// Open domain penalty seed data file as CSV
	b, _ := resources.ReadFile("seed/domain-penalties.csv")
	buf := bytes.NewBuffer(b)
	r := csv.NewReader(buf)

	// Read the header row.
	_, err = r.Read()
	if err != nil {
		return errors.Wrap(err, "missing header row in domain penalties data")
	}

	for {
		record, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return errors.Wrapf(err, "Parsing penalty CSV")
		}

		avgPenalty, err := strconv.ParseFloat(record[1], 64)
		if err != nil {
			return errors.Wrapf(err, "Parsing penalty record %s, %s", record[0], record[1])
		}
		err = db.Clauses(clause.OnConflict{ // adding this onConflict clause makes the create into an upsert
			UpdateAll: true,
		}).Create(&DomainPenalty{Domain: record[0], AvgPenalty: avgPenalty}).Error

		if err != nil {
			return errors.Wrapf(err, "Parsing inserting domain penalty %s, %f", record[0], avgPenalty)
		}

	}

	return nil
}
