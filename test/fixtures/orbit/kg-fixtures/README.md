# kg-fixtures

Knowledge Graph fixture project for the [`gitlab-mcp-server`](https://gitlab.com/jmrplens/gitlab-mcp-server) Orbit tools.

This project is intentionally simple but multi-layered to populate as many Orbit entity types as possible:

- **Source code** (Python, 7 files) → `File`, `Definition`, `Directory`, `ImportedSymbol`, `Branch`
- **CI pipeline** (`.gitlab-ci.yml`, 4 stages / 5 jobs) → `Pipeline`, `Stage`, `Job`, `JobMetadata`, `Runner`
- **Issues + labels + milestone** → `WorkItem`, `Label`, `Milestone`
- **Merge requests** with diffs → `MergeRequest`, `MergeRequestDiff`, `MergeRequestDiffFile`, `Note`
- **Releases** → `Release`

The code itself is a deliberately small `acme.orders` package that models an order-processing pipeline. Each module is small enough to read in seconds but rich enough for the Orbit indexer to extract:

- Module-level imports (cross-file `ImportedSymbol` references)
- Class definitions, method definitions, top-level function definitions (`Definition`)
- Multiple branches once the feature branch is pushed
