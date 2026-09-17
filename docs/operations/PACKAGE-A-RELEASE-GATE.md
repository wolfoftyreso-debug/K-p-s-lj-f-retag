# Package A committed release gate

Date: 2026-09-17. Status: **in progress; CI not yet passed**. P0.4 implementation has not begun.

The user's subsequent acceptance directive accepts Package A's local functionality and explicitly authorizes source review, commit, publication and clean-checkout CI. It requires this gate before P0.4. This record supplements, rather than rewrites, the earlier [local completion report](PACKAGE-A-REPORT.md) and [historical local evidence](PACKAGE-A-EVIDENCE.json). Their original source hashes identify the earlier uncommitted snapshot; later review fixes and documentation have different hashes. The release-gate commit and CI run will identify the reviewed state.

## Review and publication boundary

Review covers every repository file: application/runtime code and tests, migrations, contracts, workflow, development/verification scripts, dependency locks, doctrine and design/operations documents. Independent reviews cover persistence, runtime and tooling. Synthetic SQL identities are test setup only. Local toolchains, credentials, databases, raw workstation evidence and build binaries remain ignored under `.tools/`; the attached user image is not repository source. No production infrastructure, frontend, listing or P0.4 implementation is part of the baseline.

The existing public repository and branch `docs/phase-0-foundation` are used. Publication of the reviewed work is authorized; main-branch merge, branch-protection changes, licensing and production deployment are not part of this gate.

## Findings and corrections

The review identified an uncertain consumer COMMIT outcome: a committed receipt/projection whose COMMIT response is lost could be incorrectly classified as a terminal processing failure at the retry limit. Consumer commit failures now preserve the lease with an explicit uncertainty classification, allowing durable receipt recovery. Two PostgreSQL regressions cover committed and uncommitted outcomes with commit-boundary fault injection; this is not a claimed physical network-fault test. No architecture substitution or check suppression is authorized to obtain a passing run.

The review also found that the workstation `.tools` exclusion was incorrectly reused for Git-history secret scanning. History now uses a separate configuration without that path exclusion. Verification rejects tracked `.tools` paths before other checks. A disposable synthetic Git repository supplies negative evidence that a force-added secret-shaped fixture under `.tools` is detected by the history scan.

The original local raw reports and build hashes were preserved under the ignored `.tools/evidence-package-a-local-20260917/` directory before rerunning verification. They remain historical evidence for the original snapshot; `.tools/evidence/` now holds the most recent run.

The CI workflow runs on the implementation branch, checks out the exact requested commit, verifies a clean initial checkout and preserves bounded test/scanner reports with the commit identity. The workflow still runs the full required verification against actual PostgreSQL. Artifact upload is pinned to the reviewed `actions/upload-artifact` v4 commit; reports exclude runtime credentials and local databases. Artifact retention is 14 days for synthetic CI evidence, not a personal-data retention policy.

## Required closure evidence

- Reviewed file inventory and secret scan, with no unintended staged files.
- Exact committed Package A baseline SHA and tree SHA.
- Hosted CI run URL/ID, checked-out SHA, platform and final conclusion.
- Unit/integration/race counts, migration results, dependency/SAST/secret checks, and any failures with their root-cause correction.
- P0.4 remains gated until all applicable CI checks pass; a workflow file or workstation result cannot satisfy this condition.

Commit/run evidence is pending. No green CI result is claimed by this preparatory record.
