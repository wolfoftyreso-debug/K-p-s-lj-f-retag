# Package A committed release gate

Created: 2026-09-17. Evidence recorded: 2026-09-24. Status: **Package A release gate passed, including follow-up helper corrections and their committed Linux CI result**. The complete implementation baseline is `cad0afde0b714238ceafc91f3fd558b5161b4122`; the initial foundation commit is recorded below. P0.4's decision proposal is ready; identity implementation awaits the requested D03 decision.

The user's subsequent acceptance directive accepts Package A's local functionality and explicitly authorizes source review, commit, publication and clean-checkout CI. It requires this gate before P0.4. This record supplements, rather than rewrites, the earlier [local completion report](PACKAGE-A-REPORT.md) and [historical local evidence](PACKAGE-A-EVIDENCE.json). Their original source hashes identify the earlier uncommitted snapshot; later review fixes and documentation have different hashes. The release-gate commit and CI run will identify the reviewed state.

## Review and publication boundary

Review covers every repository file: application/runtime code and tests, migrations, contracts, workflow, development/verification scripts, dependency locks, doctrine and design/operations documents. Independent reviews cover persistence, runtime and tooling. Synthetic SQL identities are test setup only. Local toolchains, credentials, databases, raw workstation evidence and build binaries remain ignored under `.tools/`; the attached user image is not repository source. No production infrastructure, frontend, listing or P0.4 implementation is part of the baseline.

The existing public repository and branch `docs/phase-0-foundation` are used. Publication of the reviewed work is authorized; main-branch merge, branch-protection changes, licensing and production deployment are not part of this gate.

## Findings and corrections

The review identified an uncertain consumer COMMIT outcome: a committed receipt/projection whose COMMIT response is lost could be incorrectly classified as a terminal processing failure at the retry limit. Consumer commit failures now preserve the lease with an explicit uncertainty classification, allowing durable receipt recovery. Two PostgreSQL regressions cover committed and uncommitted outcomes with commit-boundary fault injection; this is not a claimed physical network-fault test. No architecture substitution or check suppression is authorized to obtain a passing run.

The review also found that the workstation `.tools` exclusion was incorrectly reused for Git-history secret scanning. History now uses a separate configuration without that path exclusion. Verification rejects tracked `.tools` paths before other checks. A disposable synthetic Git repository supplies negative evidence that a force-added secret-shaped fixture under `.tools` is detected by the history scan.

The original local raw reports and build hashes were preserved under the ignored `.tools/evidence-package-a-local-20260917/` directory before rerunning verification. They remain historical evidence for the original snapshot; `.tools/evidence/` now holds the most recent run.

The CI workflow runs on the implementation branch, checks out the exact requested commit, verifies a clean initial checkout and preserves bounded test/scanner reports with the commit identity. The workflow still runs the full required verification against actual PostgreSQL. Artifact upload used the pinned v4 action in the baseline and now pins reviewed v7 `043fb46d1a93c77aae656e7c1c64a875d1fc6a0a`, whose [action manifest](https://github.com/actions/upload-artifact/blob/043fb46d1a93c77aae656e7c1c64a875d1fc6a0a/action.yml) declares Node 24. Reports exclude runtime credentials and local databases. Artifact retention is 14 days for synthetic CI evidence, not a personal-data retention policy.

## Required closure evidence

- Reviewed file inventory and secret scan, with no unintended staged files.
- Exact committed Package A baseline SHA and tree SHA.
- Hosted CI run URL/ID, checked-out SHA, platform and final conclusion.
- Unit/integration/race counts, migration results, dependency/SAST/secret checks, and any failures with their root-cause correction.
- P0.4 remains gated until all applicable CI checks pass; a workflow file or workstation result cannot satisfy this condition.

## Committed baseline result

- Commit: `b44cf0ec28a9a7c58c05e1607dc184fac7f9ab7d`; tree: `3b708eaf7faa35711efaa9a9a45a8591b464e34d`.
- Reviewed and published inventory: 69 files, no local tools/credentials/databases/binaries. No main merge or production deployment.
- [GitHub Actions run 35170643556](https://github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/actions/runs/35170643556), attempt 1; job `105041365457`: **success**, 2026-09-17. Exact committed checkout and initially clean working tree are verified by the job, with `commit.txt` in its artifact.
- Platform: Ubuntu 24.04.5 amd64, runner image `20260907.300.1`; Go 1.27.1; actual PostgreSQL 18.6 service at the pinned image digest.
- Format, vet, module integrity, three independent builds, OpenAPI, documentation links and migration setup passed. Normal and race runs each passed seven packages, 43 top-level tests and 56 subtests, zero failures and zero skipped tests. `cmd/migrate` has no unit test files; its build and real migration command succeeded. Five top-level PostgreSQL integration tests include 22 store scenarios.
- govulncheck: no vulnerabilities found; gosec: 12 production files / 1,389 lines / zero findings / zero suppressions; worktree and two-commit history secret scans: zero findings.
- Artifact `10476308292`, downloaded and SHA-256 verified: `2f6284decf968e46a5c0cab4ee2918d5a15a833948466e69c375a046123b4d3a`. Hosted retention expires 2026-10-01; raw reports are retained locally under ignored `.tools/ci-35170643556/`. The committed [CI evidence summary](PACKAGE-A-CI.json) preserves counts, identities and individual report hashes.

The job emitted one action-runtime deprecation warning for the pinned v4 artifact uploader, which GitHub ran on Node 24. All steps succeeded. A reviewed uploader update is included with the follow-up tooling correction; this warning was not treated as a failing application test or hidden from the record.

The two persistence regression cases use real transactions with explicit commit-boundary fault injection, not a physical network interruption. The secret-gate negative proof used non-issued synthetic detector text: old history configuration missed it (exit 0); the unrestricted configuration detected exactly one finding (exit 1). The verification script also rejected force-staged `.tools` content and failed closed when Git enumeration returned 128.

## Follow-up helper verification — 2026-09-24

Local startup now rejects foreign owners, null ACLs, unsafe explicit or effective access grants (including descendants), reparse points and unreadable permissions before reading credentials. Both lifecycle helpers accept only known `pg_ctl status` results: 0 means running, 3 means stopped, all others fail. No application architecture, schema or authorization rule changed.

Eleven isolated checks passed: parsing both scripts; safe ACL; explicit/inherited/protected-descendant foreign grant rejection; junction rejection; Start/Stop status 1 and 4 failure handling; stopped idempotence. Real PostgreSQL 18.6 start/reuse/stop/repeated-stop/restart/final-stop also passed. Final status was 3, with data retained. See [tooling evidence](../../scripts/dev/TOOLING.md). Raw local proof SHA-256: `22bac2333330e69664eabe3ed8ed3643ac4849b0e3d2ecb8b284550825880d31`. Hosted Linux CI does not execute Windows ACL behavior; that proof is explicitly Windows-local.

Follow-up commit `cad0afde0b714238ceafc91f3fd558b5161b4122` passed [run 35946388447](https://github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/actions/runs/35946388447), job `107465158911`, on 2026-09-24. All required checks passed; normal/race counts remained seven packages, 43 top-level tests and 56 subtests, with zero failed or skipped tests. SAST remained zero findings across 12 files/1,389 lines, dependency scan found no vulnerabilities, and worktree/three-commit history secret scans found zero leaks. The v7 uploader completed without the earlier Node deprecation warning. Artifact `10787236244` was downloaded and verified against SHA-256 `c1734ec26edbbb30fb6897077599c0c577615ef78d1d97aec0ee701b58a05142`; [machine-readable evidence](PACKAGE-A-CI.json) records the report hashes and exact results. Subsequent documentation-only commits retain their own associated GitHub Actions checks; no commit is represented as self-attesting to a future run.
