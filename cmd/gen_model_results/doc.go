// Command gen_model_results turns what a model evaluation run observed into
// what this repository publishes: the committed record, the reference page and
// the README tables.
//
// It is the second half of a split between observing and scoring. A
// run writes observation and no verdict, so every number here is computed on
// this side, from the shards and from the corpus at HEAD, at the moment a run
// is folded in.
//
// What that buys, stated as it really stands rather than as the shard record
// states it: a scoring rule corrected today re-scores a past run without
// spending a token, and what it takes is that run's shards. The committed
// record holds the scored columns, not the observation; redrawing a page
// re-reads them and scores nothing; and a second fold of a run already
// published is refused by name rather than replacing it. So a corrected rule
// reaches a published row through one path, -refold, which merges the given
// shards into the rows they publish again, case by case, naming each case it
// replaces. The case is the unit and not the row: replacing the row is what it
// used to do, and a re-run of one corrected case then replaced a row measured
// over every case with one measured over that case alone. A run whose
// shards were not kept cannot be re-scored at all, which is the reason to keep
// them.
//
// Five flags, of which the writing ones compose:
//
//	go run ./cmd/gen_model_results/ -shards dist/modeleval/ce -render            # fold a run in and redraw
//	go run ./cmd/gen_model_results/ -shards dist/modeleval/ce -refold -render    # merge that run in again, case by case
//	go run ./cmd/gen_model_results/ -shards dist/modeleval/ce -dry-run -render   # rehearse the whole path into dist/, publishing nothing
//	go run ./cmd/gen_model_results/ -render                                      # redraw from the record alone
//	go run ./cmd/gen_model_results/ -check                                       # the offline gate
//
// It is one of the four sanctioned readers of the corpus's answer key. What it
// publishes is about the key and it produces no stimulus, which is the ground
// the corpus boundary test names it on.
package main
