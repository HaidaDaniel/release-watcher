# Task System

- One task file per unit of work.
- Each task must list context files, allowed files, acceptance criteria, and verification commands.
- Choose checks from this repository's tools and the task's changed behavior; APK does not impose its own quality pipeline.
- Use focused feedback during edits and a non-overlapping final verification set on the committed candidate; coverage already runs its measured tests.
- Documentation-only tasks need applicable contract, link, or example checks rather than unrelated application suites.
- Independent review inspects current trusted evidence and focused counterexamples; rerun broader checks when evidence is stale, failed, missing, or insufficient.
