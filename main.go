package main

import (
	"fmt"
	"os"

	"github.com/Remitly/qbert-etl/app/stores/volume"
	"github.com/Remitly/qbert-sdk/cal"
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
}
