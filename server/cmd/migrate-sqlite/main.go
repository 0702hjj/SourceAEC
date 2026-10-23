// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package main

import (
	"encoding/json"
	"flag"
	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/migration"
	"log"
	"os"
)

func main() {
	source := flag.String("source", "", "offline SQLite backup path")
	root := flag.String("data-dir", "", "target workspace directory (same as VIEWER_DATA_DIR)")
	flag.Parse()
	if *source == "" || *root == "" {
		log.Fatal("--source and --data-dir are required; stop all services before migration")
	}
	db, err := artifact.Connect(*root)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	counts, err := migration.Import(*source, db)
	if err != nil {
		log.Fatal(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(counts); err != nil {
		log.Fatal(err)
	}
}
