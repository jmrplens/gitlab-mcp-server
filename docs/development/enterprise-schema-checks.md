# Enterprise Schema Checks

**How the Premium and Ultimate surface is verified** — what runs on a schedule with
no license, and what the maintainer runs by hand before a release.

> **Diátaxis type**: How-to & Explanation · **Audience**: 🛠️ Maintainers

The Enterprise-only tools were the least-checked part of this server for a long time,
and the reason was structural rather than accidental. `.github/workflows/e2e.yml` runs
the suite with `-tags e2e` against a GitLab CE image, and the `enterprise` build tag
appears in no workflow, so 41 files and 74 test functions covering the Premium and
Ultimate surface were never compiled. Every package that turned out to hold a broken
GraphQL document was a package only that tag covers.

There are two checks, and they answer different questions.

## The scheduled re-probe: no license, weekly

`.github/workflows/ee-schema.yml` boots an **unlicensed** `gitlab/gitlab-ee:latest`,
introspects it, and validates every GraphQL document this repository sends against the
schema that instance serves. It needs no secret, no activation code, and nothing that
expires.

It works because of one fact worth stating plainly: **an unlicensed GitLab EE serves
the whole Enterprise schema.** GitLab builds its GraphQL schema when the process boots,
and a license is applied afterwards against an already-healthy instance, so the schema
exists before any license does. Introspecting a booting `gitlab/gitlab-ee:latest`
returned 4233 types, including `Vulnerability`, `VulnerabilitySeverity`,
`PipelineSecurityReportFinding`, `Epic`, `Iteration`, `MergeTrain`,
`ComplianceFramework` and `RelativePositionType`. Licensing is enforced in resolvers
and in authorization, not by removing types, and a malformed document is refused at
validation, before either of those runs.

The same run reports where the pinned schema and the live one disagree **about a type
or field one of our documents touches**. Whole-schema drift between two GitLab releases
is thousands of lines and tells nobody anything; the drift under our own selection sets
is a handful of coordinates, and it is what turns the pin's age from an assumption into
a number somebody sees.

Run the same check locally against any instance:

```bash
# gitlab.com, which answers introspection to anyone
make check-graphql-documents-live

# any other instance, including a local unlicensed EE
make check-graphql-documents-live GRAPHQL_LIVE_URL=http://localhost:8929/api/graphql
```

Two failure modes are kept apart on purpose, because a job that goes red for a boring
reason is a job nobody reads, and that is how this whole class of defect survived:

- **The instance never became something to ask.** The image did not pull, or GitLab was
  not ready inside the window. The job summary says so in those words, and nothing was
  validated. It is a job-infrastructure failure with a different owner.
- **An instance that answered with less than a whole schema.** The re-probe refuses an
  introspection carrying fewer types than a GitLab schema has, the same floor
  `make check-graphql-schema` refuses a truncated pin with. Without it, every document
  would validate against the fragment that arrived and the run would report success for
  a question nobody asked, which is worse than not running at all.

**What this job is not:** it does not run the Enterprise test suite, because that needs
a license and there is not a durable one. See below.

## The licensed run: manual, before a release

This is the only layer that verifies what a licensed feature **does**, rather than
whether it can be asked for. It stays a manual pre-release step for a reason that is a
constraint and not a preference: the Enterprise licenses available here are 30-day
trials obtained one at a time, so a CI secret holding one is a job that goes red every
month for a reason unrelated to the code.

Running it once against a licensed Ultimate also settled what it is and is not good
for. The run passed with 398 tests, one skipped and two failures, and neither failure
was a security tool while three security tools could not work at all: every document
gitlab.com refuses was executed against a real Ultimate instance and reported success.
So a scheduled licensed run would not have caught the defect class the re-probe above
exists for. It catches the other one, which is a handler that returns an empty list on
purpose and a test that only asserts there was no error.

### What it needs

- A GitLab Enterprise trial activation code, obtained from
  <https://about.gitlab.com/free-trial/>. It is a 24-character activation code, not a
  license file. `test/e2e/scripts/enterprise-activation-code.sh` finds it in
  `GITLAB_ACTIVATION_CODE`, in `ENTERPRISE_LICENSE`, or in the repository `.env`, and
  reuses a cached license at `test/e2e/.enterprise-license` when one is already there.
- Docker, and roughly 6 GB of memory for the container stack.
- A host with room to spare. The container stack belongs on a bigger idle machine with
  the Go process local and pointed at it, rather than on a laptop that is also being
  used.

### How long it takes

About ten minutes of GitLab booting, setup and activation, then about sixteen minutes
of tests. Budget half an hour end to end, and expect the boot to dominate.

### How to run it

```bash
export GITLAB_ACTIVATION_CODE=...   # the trial activation code
make test-e2e-docker-enterprise
```

That target starts GitLab EE from `test/e2e/docker-compose.yml`, waits for it, applies
the license, registers the runner, runs the suite with `-tags "e2e enterprise"` and
tears the stack down, leaving its reports under `dist/e2e-reports/`.

The `enterprise` tag is the load-bearing part. Without it the Premium and Ultimate
files are not compiled, which is the gap this page exists to close.

### Where its result belongs

The release rehearsal (`gh workflow run release.yml --ref <branch>`, described in the
release process section of [CLAUDE.md](../../CLAUDE.md)) is the moment to run it, so a
tag cannot be cut without somebody having looked at the result.

## Why not just run the suite on a schedule

Because the license does not exist. Everything above follows from that one fact:
the schema question is answerable without a license and is therefore automated, and the
behavior question is not and is therefore a ritual with a person in it.
