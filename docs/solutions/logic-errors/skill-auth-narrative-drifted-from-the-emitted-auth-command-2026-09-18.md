---
title: "The SKILL auth narrative drifted from the emitted auth command"
date: 2026-09-18
category: logic-errors
module: internal/generator
problem_type: logic_error
component: generator
related_components:
  - testing_framework
  - documentation
symptoms:
  - "SKILL.md told the reader to run `auth setup`, which exited 2 and printed the parent auth help."
  - "verify-skill reported zero findings on a SKILL.md containing a command that does not exist."
  - "The SKILL named one auth env var while the CLI required three."
root_cause: duplicated_selection_logic
resolution_type: code_fix
severity: high
tags:
  - skill
  - auth
  - verify-skill
  - narrative
---

# The SKILL auth narrative drifted from the emitted auth command

## Problem

`skill.md.tmpl` instructed readers to run `<cli> auth setup` (and `--launch`) in
all five of its auth branches. Two auth templates emit no `setup` subcommand:

| Auth template | `auth setup` |
|---|---|
| `auth.go.tmpl` | emitted |
| `auth_simple.go.tmpl` | emitted |
| `auth_device_code.go.tmpl` | emitted |
| `auth_client_credentials.go.tmpl` | **not emitted** |
| `auth_browser.go.tmpl` | **not emitted** |

So every CLI printed with client-credentials or browser auth shipped a SKILL.md
whose first instruction exits 2. The agent lands on the parent `auth` help,
where the only working paths are `login` and `set-token` — both of which
persist a secret to disk. A document meant to get an agent working safely
steered it toward the one behavior a read-only deployment forbids.

The narrative also named only `Auth.CanonicalEnvVar`, so a reader of a
three-variable auth model set one and the CLI still refused.

## Root cause

The auth template was chosen by an inline `switch` inside the render function,
and the SKILL narrative branched on `Auth.Type` separately. Two pieces of logic
answering "which auth flow is this" from different inputs will drift, and these
did: `Auth.Type` was `bearer_token` for a client-credentials grant, which is the
branch that mentions `auth setup`.

## Resolution

The selection `switch` is extracted to `authTemplateName()`, and
`authHasSetupCommand()` reports whether the chosen template emits `setup`. The
SKILL asks that helper instead of re-deriving the flow, so the two cannot drift
again. `requiredAuthEnvVars` lists every env var the model marks required.

## The gap that let it ship

`verify-skill` returned **zero findings** on this SKILL. Its
`check_unknown_commands` walks only two surfaces:

1. bash recipes extracted from prose, and
2. inline backticks inside the `## Command Reference` section.

The auth narrative lives under `## Auth Setup`, so its commands are never
checked. **The part of the SKILL an agent reads first to get working is the
least-verified prose in the document.** This is not fixed. Until it is, an auth
narrative can name any command at all and ship green.

The golden suite did not catch it either: every auth fixture uses a flow that
*does* emit `setup`, so no golden changed when the bug was introduced or when it
was fixed. New generator tests cover all four selection branches instead.

## Verification

- `TestSkillOmitsAuthSetupWhenTheAuthCommandHasNone` — asserts the instruction
  is absent, and that the emitted `auth.go` really lacks `newAuthSetupCmd`, so
  the assertion cannot pass vacuously.
- `TestSkillKeepsAuthSetupWhenTheAuthCommandHasOne` — the instruction survives
  where it is true.
- `TestAuthHasSetupCommandTracksTheSelectedTemplate` — all four branches.
- `TestSkillNamesEveryRequiredAuthEnvVar`.

## Related

Found by the Phase 4.8 agentic SKILL review, which exists precisely because
mechanical checks cannot ask "does this instruction correspond to something the
CLI can do." The same review found the Command Reference emitting headings for
resources whose commands all live in sub-resources, fixed in the same change by
recursing into `SubResources`.
