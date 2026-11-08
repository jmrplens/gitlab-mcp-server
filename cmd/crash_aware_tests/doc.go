// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command crash_aware_tests runs a test suite through gotestsum and tells a
// Go runtime crash apart from a failing test. It is item 5 of issue 1114,
// .github/scripts/go-test-crash-aware.sh moved into Go, and it is what the
// unit suite of the cross-platform job runs through on all three legs, so the
// Windows leg no longer needs bash or jq to read its own verdict.
//
//	crash_aware_tests <json-dir> [-flag=value ...] <packages ...>
//
// Flags are passed to go test as written and must carry their value in the
// same word (-timeout=30m, not -timeout 30m), because the packages are told
// from the flags by their first character and run again on their own.
//
// # What it decides
//
// The first run is gotestsum --jsonfile <json-dir>/first.json -- <args>. Its
// go test -json stream is then read back. A package whose output carries one
// of the signatures below is a crashed package. A package with a fail event,
// no signature, and no test of its own that failed is a dead package: its
// binary stopped before its tests could finish, and it left no message saying
// why. A package with a fail event and a failing test, a panic, or a build or
// setup failure is a failing package. Failing packages fail the run, as they
// always did, and are listed with their failing tests. Crashed and dead
// packages are run once more, alone: one that passes is reported as a runtime
// crash in the log and in the job summary and does not fail the run, one that
// crashes, dies or fails again does.
//
// The dead class exists because the crash does not always leave its message.
// On 2026-09-12 the Windows leg lost two packages in one run: cmd/server with
// the usual "fatal error: unexpected signal during runtime execution", and
// internal/toolutil, which stopped after 721 ms with fourteen tests still
// running, no fail event for any of them and no output at all, which is a
// process the kernel ended before the runtime could write a word. The first
// was recognized; the second was counted as a failing package, and a failing
// package stops the rerun, so the crash that was recognized was not rerun
// either and the leg failed on a runner bug with nothing to fix.
//
// # Why
//
// The Windows leg of the cross-platform matrix dies inside the Go runtime now
// and then (issue 467). golang/go#81238 explains it: on hosts with Intel AMX,
// recovering a hardware exception on a goroutine stack lets Windows write its
// exception context below the stack into the heap, and the next garbage
// collection faults. The hosted pool is heterogeneous, so the same commit
// crashes on one runner and passes on the next. Rerunning the whole job by
// hand told nobody anything and cost twenty minutes; failing on the crash
// blocked merges on a bug nobody here can fix; ignoring the leg let a real
// Windows regression pass as one more flake. This keeps the distinction, and
// keeps a record of every crash where a reader looks for one.
//
// # What a crash is
//
// A crash is a "fatal error:" line from the runtime, minus the messages the
// runtime uses for a bug in the code under test. The first version of the
// script named the corruption messages instead (a fault, a pointer to a free
// object, a bad pointer in the heap) and the third crash seen was none of
// them: "acquireSudog: found s.elem != nil in cache", the sudog cache's own
// consistency check tripping over the same corrupted heap. A corrupted heap
// trips whichever runtime invariant reads it first, so that list could never
// be complete; the list of things the runtime says about a bug in user code
// is short and stable, and that is the one kept ([bugMark]). "panic: runtime
// error" is never a crash: an unrecovered nil dereference is a bug in the
// code under test.
//
// # What changed in the port
//
// Two answers the script gave without meaning to are refused here. A first
// run that wrote no stream exited with gotestsum's own status, which is 0
// when gotestsum itself could not tell anything went wrong, so a run that
// judged nothing could pass; it now fails. And a rerun that wrote no stream
// left every package of it reported as passed when gotestsum exited 0; each
// such package is now reported as having no stream, and the run fails.
//
// A package's crash line is the first one its stream carries, where the
// script kept whichever one sort put first.
package main
