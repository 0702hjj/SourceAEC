// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
)

type notebookCell struct {
	Key    string `json:"key"`
	Status string `json:"status"`
}
type notebookResult struct {
	Path       string         `json:"path"`
	SHA256     string         `json:"sha256"`
	Definition string         `json:"definition"`
	Cells      []notebookCell `json:"cells"`
}
type notebookReport struct {
	Version   int              `json:"version"`
	Notebooks []notebookResult `json:"notebooks"`
}

var notebookHash = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func validateNotebookReport(raw []byte, expected map[string]string) error {
	if len(raw) > 1<<20 {
		return fmt.Errorf("notebook report exceeds size limit")
	}
	var report notebookReport
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF || report.Version != 1 || len(report.Notebooks) != len(expected) {
		return fmt.Errorf("invalid notebook completion report")
	}
	seen := map[string]bool{}
	cells := 0
	for _, result := range report.Notebooks {
		if seen[result.Path] || expected[result.Path] == "" || expected[result.Path] != result.SHA256 || !notebookHash.MatchString(result.Definition) {
			return fmt.Errorf("notebook completion identity mismatch")
		}
		seen[result.Path] = true
		keys := map[string]bool{}
		for _, cell := range result.Cells {
			cells++
			if cells > 10000 || len(cell.Key) == 0 || len(cell.Key) > 256 || keys[cell.Key] || (cell.Status != "ran" && cell.Status != "cached" && cell.Status != "skipped") {
				return fmt.Errorf("invalid notebook cell report")
			}
			keys[cell.Key] = true
		}
	}
	return nil
}

func notebookSummary(product Product) (map[string]any, error) {
	if len(product.NotebookReport) == 0 {
		return nil, nil
	}
	var report notebookReport
	if err := json.Unmarshal(product.NotebookReport, &report); err != nil {
		return nil, err
	}
	counts := map[string]int{"ran": 0, "cached": 0, "skipped": 0}
	sources := []map[string]string{}
	for _, notebook := range report.Notebooks {
		sources = append(sources, map[string]string{"path": notebook.Path, "sha256": notebook.SHA256, "definition": notebook.Definition})
		for _, cell := range notebook.Cells {
			counts[cell.Status]++
		}
	}
	summary := map[string]any{"policy": "simplecadapi-v1", "status": product.NotebookStatus, "cells": counts, "notebooks": sources, "reportHash": digest(product.NotebookReport)}
	if product.NotebookCache != nil {
		summary["compatibility"] = product.NotebookCache.Identity
		summary["generation"] = product.NotebookCache.Generation
	}
	return summary, nil
}
