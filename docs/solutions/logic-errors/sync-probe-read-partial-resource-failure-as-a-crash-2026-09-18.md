---
title: "The data-pipeline probe read a partial sync failure as a crash"
date: 2026-09-18
category: logic-errors
module: internal/pipeline
problem_type: logic_error
component: tooling
related_components:
  - testing_framework
symptoms:
  - "verify reported 'Data Pipeline: FAIL: sync crashed' for a CLI whose sync exits 0 and stores rows when run by hand."
  - "The probability of the false failure rose with the number of resources the CLI declares."
  - "The probe's first attempt scoped sync to the literal resource 'repos', which most CLIs never declare."
root_cause: conflated_exit_code_with_outcome
resolution_type: code_fix
severity: medium
tags:
  - verify
  - data-pipeline
  - sync
  - shipcheck
---

# The data-pipeline probe read a partial sync failure as a crash

## Problem

`cli-printing-press verify` reported `Data Pipeline: FAIL: sync crashed` for a
601-endpoint CLI. The message was wrong in three independent ways, and each had
to be peeled back before the real cause showed.

**A caution on diagnosis.** The verdict says "crashed" for every failure mode
the probe cannot name, so it invites a guess. The first guess here — partial
resource failure, supported by a hand-built mock showing `211/213 success` —
was wrong for this CLI, and measuring against a mock that was not the probe's
own mock is what made it look right. Instrument the probe inside a real
`verify` run before believing any story about this message.

## Root causes

Three defects, found in this order.

### 1. A non-zero exit was read as a crash

A generated `sync` exits non-zero when **any single resource** fails. The probe
treated any non-zero exit as a crash of the whole pipeline, so on a wide CLI one
path the mock does not serve condemned the print. The failure tracked breadth
rather than brokenness and got likelier the more resources a CLI declares.

### 2. Two of the five probe attempts kept a fixed budget

An earlier fix scaled the probe budget with resource count, but only for three
of the five attempts. Attempts 2 and 3 — the ones that pass `--db`, and so the
only ones whose store the downstream checks read — kept a hardcoded 30s. The
sync under test needed 94s, so both were killed at the deadline.

### 3. The store check assumed every CLI has `sql`

The probe asked the store for its domain tables by shelling out to `sql`. A CLI
printed from a large spec hides `sql` behind the orchestration MCP pattern, so
the query failed with "unknown command" and an empty store was inferred — which
reported a crash for every such CLI regardless of what sync did.

### The cause that actually produced this verdict

None of the above. The printed CLI carried a hand-authored environment-
consistency guard that refused any token URL outside its allowlist. `verify`'s
mock mode injects a placeholder token host, so the guard exited non-zero on
**every** command, sync included, before a single request was made. The
generated write gate already exempted the verification harness; the
hand-authored guard did not, and that asymmetry is the whole bug. See
[`printed-cli-safety-gates-must-exempt-the-verify-harness-2026-09-18.md`](printed-cli-safety-gates-must-exempt-the-verify-harness-2026-09-18.md).

Fixing the guard moved verify from 91% to 100% (462/463) — the guard had been
failing 42 other commands too.

## Resolution

The scoped attempt now uses a resource the CLI actually declares, read from its
own `defaultSyncResources`, falling back to the historical literal only when the
declarations cannot be read.

When every attempt exits non-zero, the probe no longer guesses from the exit
code. It asks the store whether sync created its domain schema. Schema present
means the pipeline ran, so the existing row checks deliver the verdict on
evidence; absent means nothing ran and `FAIL: sync crashed` stands. Either way
the detail line says a partial resource failure occurred, so a reader seeing
PASS knows some resources never synced and a reader seeing FAIL does not go
hunting a crash that did not happen.

This deliberately does not soften the gate: a sync that writes schema but no
rows still fails, with the row counts rather than the word "crashed".

## Verification

- `TestRunDataPipelineTestReportsPartialResourceFailureRatherThanACrash`
- `TestRunDataPipelineTestStillFailsWhenSyncLeavesNoSchema` — the escape hatch
  does not swallow a real crash
- `TestRunDataPipelineTestFailsPartialSyncThatWroteNoRows` — schema without rows
  is still a failure
- `TestSyncProbeResourcePrefersADeclaredResourceOverTheGitHubFallback`

## Related

A separate defect in the same probe — a fixed 30s budget for a sequential walk
of every declared resource — was fixed first and is easy to confuse with this
one. Against a real API the budget ran out and looked like this bug; in mock
mode the failure is fast and is this one. The budget now scales with resource
count and a deadline kill is reported as a timeout.
