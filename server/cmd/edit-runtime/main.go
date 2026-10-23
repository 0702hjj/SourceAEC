// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package main

import (
	"fmt"
	"ifcviewer/server/internal/editruntime"
	"os"
)

func main() {
	serve := editruntime.Serve
	if len(os.Args) == 2 && os.Args[1] == "--tasks" {
		serve = editruntime.ServeTasks
	}
	if err := serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
