---
title: "Example coverage counted commands the generator cannot give an example"
date: 2026-09-18
category: logic-errors
module: internal/pipeline
problem_type: logic_error
component: tooling
related_components:
  - generator
  - testing_framework
symptoms:
  - "Dogfood failed with a low example-coverage percentage on wide CRUD CLIs no matter how the print was generated."
  - "The failing percentage did not move between shipcheck loops, because no generator or narrative work could change it."
  - "Commands needing an opaque resource id or a request body counted against coverage even though emitting an example for them would produce a command that fails when run."
root_cause: incorrect_metric_denominator
resolution_type: code_fix
severity: medium
tags:
  - dogfood
  - example-coverage
  - cobra-annotations
  - scorer
  - shipcheck
---

# Example coverage counted commands the generator cannot give an example

## Problem

Dogfood's example-coverage leg samples commands, reads each one's `--help`, and
fails the gate when fewer than half carry a Cobra `Example:`. On a CLI printed
from a large CRUD spec the leg failed every time and the percentage never moved.

A 601-endpoint print measured 35% coverage. The breakdown explains why it was
stuck:

| Commands lacking an example | Count | Why |
|---|---:|---|
| Mutating verbs (POST/PATCH/PUT/DELETE) | 251 | need a request body no spec value supplies |
| GET addressed by a resource id | 126 | need an id that only a prior list call yields |
| GET needing nothing but the tenant | 8 | genuinely missing |

So 377 of the 385 gaps were structural. The ceiling on honestly-derivable
coverage was roughly 36%, below a threshold of 50%.

## Root cause

The generator is right to decline these. `exampleLine` returns empty when
`requiredInputsAreDerivable` is false, and `runnableExampleLine` discards any
candidate carrying a placeholder. That is deliberate: verify and dogfood execute
the examples they find, so a synthesized `--id <id>` would ship a `--help` entry
that fails the moment anyone runs it — and the same leg would then flag it as a
broken example.

The gate, meanwhile, divided by every sampled command. It therefore measured the
shape of the API rather than the quality of the print, and on a wide surface it
could not be satisfied by any amount of generator work. A gate that cannot be
satisfied is one operators learn to ignore, which costs more than the signal is
worth.

## Resolution

The generator now emits `pp:no-runnable-example: "true"` on commands where it
declined to synthesize an example, so the decision is explicit instead of being
inferred from a missing field. Dogfood filters those commands out **before**
sampling — sampling first would spend the ten-command budget on commands that
cannot carry an example — and reports the excluded count alongside the ratio.

The gate keeps its teeth. A command with derivable inputs carries no annotation,
stays in the denominator, and still fails the check when it has no example. On
the print above the measure becomes 216/224 eligible commands, and the 8 real
gaps stay visible instead of being lost among 377 that were never fixable.

## Verification

- `TestGeneratedCommandAnnotatesWhenNoRunnableExampleIsDerivable` asserts the
  emitted annotation on an id-addressed read, its absence on a bare collection
  read, and compiles the generated module.
- `TestDiscoverExampleCheckCommandsExcludesCommandsWithNoDerivableExample`,
  `...KeepsUnannotatedCommands`, and `...FiltersBeforeSampling` cover the
  verifier side.
- `TestExampleCoverageRuleCannotFireWhenNothingIsEligible` locks the guard that
  stops the exclusion from becoming a new way to fail.

## Related

Same shape as the error-path opt-out in
[`dogfood-soft-failure-error-path-opt-out-2026-05-22.md`](dogfood-soft-failure-error-path-opt-out-2026-05-22.md):
a check whose verdict was right in general needed the generator to tell it which
commands it does not apply to.

`x-happy-args` does **not** unlock an example. It feeds `pp:happy-args`, while
example synthesis goes through `synthesizeHappyArgTokens`, which does not read
`Endpoint.HappyArgs`. A spec author who supplies happy args for an opaque id
still gets no example, and the command is still counted ineligible. Wiring the
two together is a reasonable follow-up, not part of this fix.
