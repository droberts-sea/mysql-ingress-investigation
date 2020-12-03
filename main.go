package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/Remitly/qbert-etl/app/stores/volume"
	"github.com/Remitly/qbert-sdk/cal"
	"github.com/Remitly/qbert-sdk/db"
	"go.uber.org/zap"
)

func main() {
	fmt.Println("hello world")

	ts, err := cal.ParseSqlTimestamp("2020-04-02 05:16:08.987")
	if err != nil {
		fmt.Printf("Could not parse timestamp: %v\n", err.Error())
		os.Exit(1)
	}

	vol := volume.Volume{
		TransactionId:      "abc123",
		OldestCancellation: &ts,
	}

	fmt.Printf("Oldest cancellation: '%+v'\n", vol.OldestCancellation)
	fmt.Printf("Oldest cancellation.String(): '%v'\n", vol.OldestCancellation.String())
	fmt.Printf("Table records: '%v'\n", vol.TableRecord())

	fmt.Printf("Oldest cancellation cast to time: %v\n", time.Time(*vol.OldestCancellation))

	conn := connect()
	conn.Exec(
		"INSERT INTO test_table (label, time_seconds, time_millis) VALUES (?, ?, ?)",
		"Go INSERT pass direct",
		vol.OldestCancellation,
		vol.OldestCancellation,
	)
	conn.Exec(
		"INSERT INTO test_table (label, time_seconds, time_millis) VALUES (?, ?, ?)",
		"Go INSERT call .String()",
		vol.OldestCancellation.String(),
		vol.OldestCancellation.String(),
	)
}

func connect() *sql.DB {
	log := zap.L().Sugar()

	config, err := db.Config(
		"localhost",
		"3306",
		"costbasis",
		"root",
		"password",
	)
	if err != nil {
		log.Errorw(
			"Failed to create database configuration",
			"error", err,
		)
		os.Exit(1)
	}

	conn, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		log.Errorw(
			"Failed to open database connection",
			"dbName", config.DBName,
			"error", err,
		)
		os.Exit(1)
	}

	err = conn.Ping()
	if err != nil {
		log.Errorw(
			"Failed to ping database",
			"dbName", config.DBName,
			"error", err,
		)
		os.Exit(1)
	}

	return conn
}
