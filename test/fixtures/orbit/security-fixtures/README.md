# security-fixtures

Intentionally insecure fixture project used to populate the Orbit knowledge graph's `security` domain.

The code in `src/acme/orders/` contains:

- **Hard-coded AWS and GitHub credentials** in `__init__.py` (regex-detected by Secret Detection)
- **SQL injection vectors** in `legacy_db.py` (f-string and concatenation; flagged by SAST)
- **Weak password hashing** (`md5`) and an `eval()` call in `auth.py` (flagged by SAST)

The `.gitlab-ci.yml` includes every GitLab-managed security template so the analyzers run on every push:

- `Security/SAST.gitlab-ci.yml` → emits `Finding` records
- `Security/Secret-Detection.gitlab-ci.yml` → emits `Finding` records for leaked secrets
- `Security/Dependency-Scanning.gitlab-ci.yml` → emits `Finding` records for vulnerable gems
- `Security/Container-Scanning.gitlab-ci.yml` → emits `Finding` records (only when a container image is built)

This is **not** production code. It exists solely to exercise the Orbit knowledge graph entity types `Vulnerability`, `Finding`, `SecurityScan`, `VulnerabilityIdentifier`, `VulnerabilityOccurrence`, and `VulnerabilityScanner` against a real GitLab.com instance.
