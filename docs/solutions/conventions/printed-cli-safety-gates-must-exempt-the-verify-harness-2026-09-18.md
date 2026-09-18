---
title: "Hand-authored printed-CLI safety gates must exempt the verify harness"
date: 2026-09-18
category: conventions
module: internal/pipeline
problem_type: design_pattern
component: tooling
related_components:
  - generator
  - testing_framework
symptoms:
  - "Nearly every command in verify's matrix failed at the same early stage with an identical config error."
  - "The data-pipeline probe reported 'sync crashed' although sync never made a request."
  - "A printed CLI passed its own unit tests and behaved correctly by hand, but failed verify wholesale."
root_cause: harness_unaware_guard
resolution_type: convention
severity: medium
tags:
  - verify
  - printed-cli
  - safety-gate
  - hand-authored-patch
  - harness
---

# Hand-authored printed-CLI safety gates must exempt the verify harness

## The rule

A hand-authored gate added to a printed CLI — one that can refuse a command
before it reaches the API — must stand down when `cliutil.IsVerifyEnv()` is
true, unless the gate is the thing being verified.

The generated transport gate already does this: under `PRINTING_PRESS_VERIFY=1`
mutating verbs short-circuit rather than dial. A hand-authored gate that skips
the same exemption is stricter than the surface the harness was built against,
and the asymmetry is invisible until the whole matrix goes red.

## Why it bites so hard

`verify` points the CLI at a local mock and injects placeholder configuration —
base URLs, token URLs, credentials that are deliberately not real endpoints. A
guard that validates configuration against an allowlist of real hosts will
reject those placeholders and refuse **every** command, including the probes
that are supposed to exercise other behavior.

The damage is not proportionate to the gate. One config-layer guard failing
early makes the matrix report failures in unrelated dimensions:

- the data-pipeline probe reports `sync crashed`, because sync did exit non-zero
- every command scores 1/3 or 0/3 at the same stage
- the pass rate drops far enough to fail the leg

None of those point at the guard. In the case that produced this doc, a single
config guard cost 42 command failures and a false `sync crashed`, and the
diagnosis went through three unrelated probe defects before reaching it.

## Diagnosis

When a large fraction of verify's matrix fails at the same stage with the same
error, suspect a config-layer or transport-layer refusal before suspecting the
probes. The fastest confirmation is to run one failing command by hand with
`PRINTING_PRESS_VERIFY=1` set and read the error, which the matrix summarizes
away.

## Applying it

Exempt at the gate's entry point, not at its call site, so the exemption travels
with the gate and survives regeneration:

```go
func checkSomething(...) error {
    // Stand down under the verification harness, which points the CLI at a
    // local mock and injects placeholder values that are deliberately not
    // real endpoints. A mock that never leaves the machine cannot cause the
    // harm this gate exists to prevent.
    if cliutil.IsVerifyEnv() {
        return nil
    }
    ...
}
```

Then cover both sides in the gate's own tests: that it stands down with the
harness variable set, and that it still refuses everything it refused before
when the variable is absent. The exemption removes the gate from verify's
coverage, so its unit tests become the only proof it still works.

Do **not** widen the gate's allowlist to admit the harness's placeholder values.
That weakens the gate in production to satisfy a test harness, and leaves the
placeholders looking like legitimate endpoints to anyone reading the list.

## Related

`IsDogfoodEnv()` is a different signal with a different meaning: it bounds
expensive work during a real-API matrix, and must not be used to skip network
calls. `IsAnyHarness()` covers both and is the right check for side-effecting
commands that must not act under any harness.
