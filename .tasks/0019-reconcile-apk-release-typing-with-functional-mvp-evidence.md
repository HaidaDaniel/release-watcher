# Task 0019 - Reconcile APK release typing with functional-MVP evidence

State: done
Owner: opencode-primary
Mode: mvp
Lane: planning
Type: docs
Scope: apk,process,docs
Risk: medium
Parallel: false
Depends on: 0016
Tags: docs

## Goal

Document the APK v0.5.2 release-task mismatch for completed task 0016 without rewriting its history or fabricating CI/tag/deploy evidence, and define the correct typing/checks for the future deployment task.

## Context files

- AGENTS.md
- docs/decisions.md
- docs/progress.md
- docs/roadmap.md
- .tasks/0016-mvp-acceptance-and-operational-readiness.md
- .tasks/0008-whoosh-core-deployment-spike.md

## Files allowed to edit

- docs/**
- README.md
- .tasks/**

## Files forbidden to edit

- internal/**
- cmd/**
- migrations/**
- deploy/**
- scripts/**

## Steps

1. Inspect APK release policy and 0016 evidence; record what 0016 actually verified and what it did not; add an ADR or process note; state the correct type and evidence for 0008; keep 0008 blocked and 0009 in backlog.

## Acceptance criteria

- The process note states 0016 was functional-MVP acceptance
- not a published release/deploy; no CI/tag/post-tag install is claimed; 0016 is not rewritten; 0008 remains blocked with the correct deployment-type/manual-live expectations; 0009 remains backlog; docs/progress and decisions agree; pnpm exec apkit lint passes.

## Verification

- `{"id":"apk-lint","type":"automated","required":true,"environment":"local","profile":"deterministic","apkOperation":"lint"}`

## Documentation updates

- docs/decisions.md
- docs/progress.md
- docs/roadmap.md

## Notes

- Do not create a production tag or release. Do not edit completed task evidence. One compact documentation correction is enough.
