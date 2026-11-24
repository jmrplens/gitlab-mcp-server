// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command sonar_scan runs the SonarCloud analysis locally, the way CI does,
// and reads its quality gate:
//
//	sonar_scan            # make sonar
//	sonar_scan -no-scan   # make sonar-status: the gate of the last analysis
//
// It runs in four steps. The unit tests run with coverage exactly as CI's
// coverage job runs them (go test
// -coverpkg=./cmd/...,./internal/... -coverprofile=coverage.out), and a total
// below the minimum stops there. The analysis is uploaded with the
// sonar-scanner on PATH, whose configuration is sonar-project.properties. The
// Compute Engine task the upload started is waited for. And the quality gate
// of the branch is printed, with each condition and the key measures, its
// status being the command's: 0 when the gate is OK, and 1 when it is not.
// -no-scan does the last step alone.
//
// SONARQUBE_TOKEN is read from the .env at the repository's root, which has to
// be there, or from the environment when the file does not set it. The other
// settings come from the environment:
//
//	SONAR_BRANCH         the branch analyzed and reported (the current one);
//	                     one .env sets wins
//	SONAR_HOST_URL       the SonarQube host (https://sonarcloud.io)
//	SONAR_POLL_TIMEOUT   seconds the Compute Engine task is waited for (240)
//	SONAR_POLL_INTERVAL  seconds between two looks at it (5)
//	COVERAGE_MIN         the minimum total coverage, as CI has it (90)
//
// It was scripts/sonar-scan.sh until issue 1114, whose recorded runs the tests
// replay (testdata/sonar_runs.json). These departures from it are deliberate:
//
//   - .env is parsed rather than sourced, so it runs nothing and a variable in
//     a value is one the file itself set before it, and a line that is no
//     assignment is refused, with a message that shows nothing of the file.
//   - What .env sets reaches sonar-scanner and no longer the tests, which CI
//     runs without it: a token written there for the e2e suite is no business
//     of a unit test.
//   - A flag or an argument the command does not know is a usage error rather
//     than a full scan.
//   - SONAR_POLL_TIMEOUT and SONAR_POLL_INTERVAL are whole numbers of seconds,
//     where the interval was whatever sleep took, 0.5 or 1m among them.
//   - COVERAGE_MIN is a number, where one awk could not read passed the check.
//   - The three are read before anything runs, with -no-scan too, which the
//     script ran without looking at them.
//   - What fails without a status to pass on, which is git, a request to
//     SonarCloud, a missing sonar-scanner or a properties file without its
//     line, is 1 with a message of one line, where the script ended with the
//     status of git, of curl or of the shell, and said what they said.
//   - The title of the report carries no em dash.
package main
