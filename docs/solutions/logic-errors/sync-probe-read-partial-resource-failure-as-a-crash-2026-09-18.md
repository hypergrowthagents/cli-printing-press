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
601-endpoint CLI that syncs cleanly against a healthy mock:

```
sync --full -> exit 0, 213 resources, 211 success, 2 warned, 0 errored, 24.1s
```

## Root cause

Two behaviours combined.

A generated `sync` exits non-zero when **any single resource** fails, which is
reasonable on its own. `runDataPipelineTest` then treated any non-zero exit as a
crash of the whole pipeline. With hundreds of resources, one path the
spec-derived mock does not serve was enough to condemn the print — so the
failure tracked breadth rather than brokenness, and got likelier the wider the
CLI.

The probe's first attempt made this worse by scoping sync to the literal
resource name `repos`, a GitHub-ism absent from nearly every other CLI. That
attempt failed by construction on all of them, spending a probe and teaching
nothing.

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
