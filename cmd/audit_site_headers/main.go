// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// toolName is how every message this command prints names its source.
const toolName = "audit_site_headers"

// defaultSiteURL is the published documentation site's root, judged when no
// -url is given. It is the page the Best Practices form's answer links.
const defaultSiteURL = "https://docs.jmrp.io/gitlab-mcp-server/"

// osExit is a seam over os.Exit, so a test can observe the exit code main
// passes on without ending the test process.
var osExit = os.Exit

func main() {
	osExit(runMain(os.Args[1:], os.Stdout, os.Stderr, httpFetch(newHTTPClient()), time.Sleep))
}

// pageURLs collects the repeated -url flag.
type pageURLs []string

// String returns the URLs as the flag package prints a default.
func (u *pageURLs) String() string { return fmt.Sprint([]string(*u)) }

// Set adds one URL.
func (u *pageURLs) Set(value string) error {
	*u = append(*u, value)
	return nil
}

// runMain parses the command line, fetches and judges each page, prints the
// report on stdout and appends it to -summary when one is named. It returns 2
// for a usage error, 1 when a required header does not hold, a page was not
// judged or the summary could not be written, and 0 otherwise.
func runMain(args []string, stdout, stderr io.Writer, fetch fetchFunc, wait func(time.Duration)) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	var urls pageURLs
	flags.Var(&urls, "url", "a page to judge, repeatable (default "+defaultSiteURL+")")
	attempts := flags.Int("attempts", 5, "how many times a page is fetched while it fails or answers 5xx")
	delay := flags.Duration("wait", 10*time.Second, "the delay between two attempts")
	annotate := flags.Bool("annotate", false, "print a GitHub Actions workflow command for every line that is not ok")
	summary := flags.String("summary", "", "a file to append the report to as Markdown, such as $GITHUB_STEP_SUMMARY")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "%s: takes no arguments, and was given %q; name each page with -url\n", toolName, flags.Args())
		return 2
	}
	if len(urls) == 0 {
		urls = pageURLs{defaultSiteURL}
	}
	for _, each := range urls {
		if parsed, err := url.Parse(each); err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			_, _ = fmt.Fprintf(stderr, "%s: -url %q is not an https URL with a host; the headers judged are the ones a browser reads over https\n", toolName, each)
			return 2
		}
	}
	if *attempts < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: -attempts is %d, and the page has to be fetched at least once\n", toolName, *attempts)
		return 2
	}
	if *delay < 0 {
		_, _ = fmt.Fprintf(stderr, "%s: -wait is %v, and a delay cannot be negative\n", toolName, *delay)
		return 2
	}
	pages := make([]page, 0, len(urls))
	for _, each := range urls {
		status, header, err := fetchWithRetry(context.Background(), fetch, each, *attempts, *delay, wait)
		fetched := page{url: each, status: status, err: err}
		if fetched.judged() {
			fetched.verdicts = judgePage(header, siteHeaders)
		}
		pages = append(pages, fetched)
	}
	writeReport(stdout, pages, *annotate)
	code := 0
	if failed(pages) {
		code = 1
	}
	if *summary != "" {
		if err := appendSummary(*summary, pages); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: write summary: %v\n", toolName, err)
			return 1
		}
	}
	return code
}

// appendSummary appends the Markdown report to path. It appends rather than
// replaces because the path a workflow passes is $GITHUB_STEP_SUMMARY, which
// every step of a job writes its own section to.
func appendSummary(path string, pages []page) error {
	// #nosec G304 -- the path is the one the caller named on the command line.
	file, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writeSummary(file, pages)
	return file.Close()
}
