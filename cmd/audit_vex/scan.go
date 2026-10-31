// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/vuln/scan"
)

// findingKey names an advisory against one module.
type findingKey struct {
	osv, module string
}

// scanResult is what one source scan said about the dependency set.
type scanResult struct {
	db         string
	dbModified string
	// levels holds, for every advisory against every module the scan
	// reported, the deepest level it reached.
	levels map[findingKey]level
	// versions holds the version of the module each finding was reported
	// against, as the scan names it.
	versions map[findingKey]string
	// summaries holds the one-line summary of every advisory the stream
	// carried, by id.
	summaries map[string]string
	// modules holds the version of every module the loaded packages come
	// from, by path, as the stream's SBOM names them: the modules the scan
	// can speak for, whether or not it reports anything against them.
	modules map[string]string
}

// scanMessage is one object of govulncheck's JSON stream, reduced to the
// fields this command reads. The stream's schema is the Message type of
// golang.org/x/vuln/internal/govulncheck, protocol v1.0.0.
type scanMessage struct {
	Config *struct {
		DB             string `json:"db"`
		DBLastModified string `json:"db_last_modified"`
		ScanMode       string `json:"scan_mode"`
	} `json:"config"`
	SBOM *struct {
		Modules []struct {
			Path    string `json:"path"`
			Version string `json:"version"`
		} `json:"modules"`
	} `json:"SBOM"`
	OSV *struct {
		ID      string `json:"id"`
		Summary string `json:"summary"`
	} `json:"osv"`
	Finding *struct {
		OSV   string `json:"osv"`
		Trace []struct {
			Module   string `json:"module"`
			Version  string `json:"version"`
			Package  string `json:"package"`
			Function string `json:"function"`
		} `json:"trace"`
	} `json:"finding"`
}

// scanSource runs govulncheck over the packages cfg names, in source mode
// and at symbol level, which is the scan make govulncheck runs.
//
// It runs in this process rather than as a child, so the scanner is the
// version go.mod pins and nothing on PATH can stand in for it.
func scanSource(ctx context.Context, cfg auditConfig) (scanResult, error) {
	args := []string{"-C", cfg.dir, "-format=json", "-db=" + cfg.db}
	if cfg.tags != "" {
		args = append(args, "-tags="+cfg.tags)
	}
	var stdout, stderr bytes.Buffer
	cmd := scan.Command(ctx, append(args, cfg.patterns...)...)
	cmd.Stdin = strings.NewReader("")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := errors.Join(cmd.Start(), cmd.Wait()); err != nil {
		return scanResult{}, fmt.Errorf("govulncheck over %s: %w: %s", strings.Join(cfg.patterns, " "), err, strings.TrimSpace(stderr.String()))
	}
	return parseScan(&stdout)
}

// parseScan reads govulncheck's JSON stream.
//
// A stream without its configuration, or the configuration of another mode,
// is refused rather than read as a clean scan: empty output and a scan that
// found nothing would otherwise look the same, and only one of them is a
// dependency set somebody checked. A stream without its SBOM is refused for
// the same reason: without it nothing says which modules the scan loaded,
// and a statement about a module it did not load would read as stale or as
// held depending on a message that was never sent.
func parseScan(r io.Reader) (scanResult, error) {
	result := scanResult{levels: map[findingKey]level{}, versions: map[findingKey]string{}, summaries: map[string]string{}}
	mode := ""
	decoder := json.NewDecoder(r)
	for {
		var msg scanMessage
		err := decoder.Decode(&msg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return scanResult{}, fmt.Errorf("reading govulncheck's output: %w", err)
		}
		if msg.Config != nil {
			result.db, result.dbModified, mode = msg.Config.DB, msg.Config.DBLastModified, msg.Config.ScanMode
		}
		if msg.SBOM != nil {
			result.modules = map[string]string{}
			for _, module := range msg.SBOM.Modules {
				result.modules[module.Path] = module.Version
			}
		}
		if msg.OSV != nil {
			result.summaries[msg.OSV.ID] = msg.OSV.Summary
		}
		if msg.Finding != nil && len(msg.Finding.Trace) > 0 {
			frame := msg.Finding.Trace[0]
			key := findingKey{osv: msg.Finding.OSV, module: frame.Module}
			reached := levelModule
			switch {
			case frame.Function != "":
				reached = levelSymbol
			case frame.Package != "":
				reached = levelPackage
			}
			result.levels[key] = max(result.levels[key], reached)
			result.versions[key] = frame.Version
		}
	}
	if mode != "source" {
		return scanResult{}, fmt.Errorf("govulncheck's output carries the configuration of a %q scan, and this command reads a source scan", mode)
	}
	if result.modules == nil {
		return scanResult{}, errors.New("govulncheck's output carries no SBOM, so nothing says which modules the scan loaded")
	}
	return result, nil
}
