# Task 0009 - Compare Whoosh Core with the ResLedger deployment system

State: todo
Owner: none
Mode: mvp
Lane: research
Scope: deployment,research,docs
Risk: low
Parallel: false
Depends on: 0008
Tags: none

## Goal

After the Whoosh spike succeeds, compare Whoosh Core with the deployment system used by ResLedger on reliability, own code volume, migration support, atomic deploy, rollback, backup, setup complexity, maintainability and reuse. Record a recommendation without rewriting ResLedger's deploy system.

## Context files

- AGENTS.md
- docs/deployment.md
- docs/decisions.md

## Files allowed to edit

- docs/deployment.md
- docs/decisions.md
- docs/progress.md

## Files forbidden to edit



## Steps

1. Read the ResLedger deploy scripts and manifest flow without changing that repository.
2. Compare reliability, custom code, migrations, atomic swap, rollback, backup, setup and maintainability against the recorded Whoosh spike.
3. Record a recommendation and any follow-up task; do not rewrite ResLedger deployment.

## Acceptance criteria

- The comparison uses observed evidence from both systems, not claims from READMEs alone.
- A clear recommendation and explicit unresolved risks are recorded.

## Verification

- `{"id":"apk-lint","type":"automated","required":true,"environment":"local","profile":"deterministic","apkOperation":"lint"}`

## Documentation updates

- Record the comparison in docs/deployment.md and docs/decisions.md.

## Notes

- This task exists only after a successful Whoosh spike and first deployment. It must not modify ResLedger.
