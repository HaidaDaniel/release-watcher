# Domain model

Status: implemented by migrations 0001–0002 and `internal/store`, with source adapters and scan logic implemented and tested.

## Entity overview

```text
Work 1---* Publication 1---* Release
              | 1
              *
            Source
              |
              | 1
              *
            Watch 1---* ScanRun
              |
              *---* Event 1---* NotificationDelivery
```

One Work may have several Publications (WN, LN, translation, another source). One Publication belongs to one Source line and has at most one active Watch in the MVP. A Release belongs to exactly one Publication. Events are publication-scoped, not work-global, so different lines are never summed.

## Work

A logical work, for example "The Dreaming Boy Is a Realist".

| Field | Meaning | Origin |
| --- | --- | --- |
| `id` | internal stable id | computed |
| `original_title` | title in the source language | source fact |
| `title_ru`, `title_en` | user/agent supplied localized titles, nullable | user fact |
| `created_at`, `updated_at` | local timestamps | computed |

Localized titles are stored and reused. Nothing is re-translated during a scan. If a localized title is absent, the CLI renders the original; it never fabricates a translation.

## Publication

A concrete publication line: a WN on Narō, an LN edition, a translation, a mirror. Similar titles never merge lines automatically.

| Field | Meaning | Origin |
| --- | --- | --- |
| `id` | internal id | computed |
| `work_id` | owning Work | relation |
| `publication_type` | `wn`, `ln`, `translation`, `other`, `unknown` | user or adapter fact |
| `original_url` | canonical source URL | adapter fact |
| `source_id` | Source row | relation |
| `status` | `ongoing`, `completed`, `unknown` | adapter fact when provable |
| `status_source` | which adapter/field asserted it | provenance |
| `last_published_at` | latest known release publication time, nullable | computed from releases |
| `last_checked_at` | last successful check, nullable | computed |
| `next_check_at` | next due time | computed |
| `release_count` | last known release count, nullable | adapter fact |
| `created_at`, `updated_at` | local timestamps | computed |

`status = completed` is only stored when the source exposes an explicit completion field. For Narō, the official `end` field maps `0` to completed/short story and `1` to ongoing; missing, null or unsupported values stay `unknown`. Cessation of updates is not completion. `status_source` makes the difference auditable. Absent/unknown values stay nullable.

## Source

The external site, API or feed line.

| Field | Meaning |
| --- | --- |
| `id` | internal id |
| `kind` | `naro`, `rss`, `atom` |
| `uri` | canonical feed/API/catalog URL |
| `external_id` | stable source identity (for example `N2166FE`) |
| `fields_available` | JSON list of fields the adapter can prove for this fetch |
| `created_at`, `updated_at` | local timestamps |

`fields_available` is diagnostic output, not a claim that every scan will return those fields.

## Watch

A specific observation of a Publication.

| Field | Meaning |
| --- | --- |
| `id` | internal id used by CLI |
| `publication_id` | observed Publication |
| `enabled` | whether `scan --due` includes it |
| `interval_seconds` | per-watch check interval; defaults to the global interval |
| `baseline_established_at` | when the first snapshot was stored |
| `baseline_count` | count stored by the baseline (count-only mode) |
| `mode` | `catalog` or `count_only` |
| `next_check_at` | next due time |
| `last_check_at` | last attempt |
| `last_success_at` | last successful fetch/compare |
| `last_error_category` | bounded error category, no secrets |
| `consecutive_failures` | retry/health signal |
| `paused_at` | nullable; set while paused |
| `removed_at` | nullable; set when the user stops watching while history is preserved |
| `created_at`, `updated_at` | local timestamps |

Pause/resume changes only scheduling. `remove`/unwatch sets `removed_at`, disables scheduling and preserves the publication line, releases, events, deliveries and scan runs; event ids stay stable and history export keeps working. Re-adding the same URL restores the same watch and does not duplicate releases or replay already-known chapters. `purge` is the explicit destructive operation: it deletes the observation, its publication line, releases, events, deliveries and scan runs in one transaction. Work and Source rows are removed only when no other publication line still references them. Neither operation silently modifies another publication line.

## Release

One published unit: a chapter, volume, book or other serialized item.

| Field | Meaning | Origin |
| --- | --- | --- |
| `id` | internal id | computed |
| `publication_id` | owning Publication | relation |
| `external_id` | stable source identity, unique per publication | adapter fact |
| `kind` | `chapter`, `volume`, `book`, `entry`, `other` | adapter fact |
| `number` | display ordinal as text, nullable | adapter fact |
| `title` | release title, nullable | adapter fact |
| `url` | release URL, nullable | adapter fact |
| `published_at` | source publication time, nullable | adapter fact |
| `source_updated_at` | source last-edit time, nullable | adapter fact |
| `content_hash` | bounded fingerprint of identity-relevant fields | computed |
| `is_baseline` | inserted during initial baseline | computed |
| `first_seen_at`, `last_seen_at` | observation timestamps | computed |
| `removed_at` | set only when the source provides authoritative removal data | source fact |

`external_id` is preferred over a mutable count. Narō chapters use `<ncode>/<chapter-number>`; RSS entries use guid/id, then link, then a hash. A count-only source has no Release rows until the adapter can identify them.

## Event

An immutable change detected during a scan. Events are stored even when every notifier is unavailable or unconfigured.

| Field | Meaning |
| --- | --- |
| `id` | event id used by consumers |
| `event_type` | `release.added`, `release.updated`, `release.removed`, `publication.metadata_changed`, `publication.status_changed`, `release.count_changed` |
| `publication_id`, `watch_id`, `source_id` | provenance |
| `external_work_reference` | stable source reference |
| `release_ids` | JSON array of release external ids (may be empty in count-only mode) |
| `previous_count`, `current_count`, `count_delta` | count-only evidence |
| `payload` | versioned JSON snapshot of the change |
| `dedupe_key` | unique key that prevents replayed duplicates |
| `schema_version` | event contract version |
| `detected_at`, `published_at` | detection and source time |
| `url` | best available original URL |
| `created_at` | local insertion time |

Events are immutable. Repeated reads return the same `id`. Consumers deduplicate by `id`/`dedupe_key` and can page by `detected_at` plus `id`.

## NotificationDelivery

A durable attempt to deliver one event over one channel.

| Field | Meaning |
| --- | --- |
| `id` | internal id |
| `event_id` | source event |
| `channel` | `telegram` in the MVP |
| `status` | `pending`, `sent`, `failed` |
| `attempts` | attempts made |
| `next_attempt_at` | retry schedule |
| `last_error` | bounded, secret-free error |
| `sent_at` | success time |

A unique `(event_id, channel)` constraint prevents duplicate deliveries. Retrying a failed delivery never creates a second event.

## ScanRun

History of checks and their outcome: started/finished time, watch/publication, result (`baseline`, `ok`, `error`, `unchanged`), events created, bounded error category. Errors of one watch are recorded without aborting the rest.

## Baselines and comparison rules

1. The first successful observation of a watch is a baseline: releases are stored with `is_baseline = 1` and no release events are emitted.
2. For `catalog` snapshots, compare by `external_id`, then by identity-relevant fields. New ids produce `release.added`; changed ids produce `release.updated`; missing ids produce `release.removed` only when the adapter declares the catalog authoritative and complete.
3. For RSS, missing old entries never imply removal because feeds are sliding windows.
4. For `count_only`, store the count and emit a count event only when the count changes. Never invent numbers or URLs.
5. A count-only baseline that later gains a complete catalog is reconciled silently (no bulk "new" event) before normal catalog detection resumes; releases absent from the recovered complete catalog are marked removed silently, and a partial catalog recovery stays in count-only mode and emits no events. Duplicate external ids in one snapshot are collapsed before storage.
6. Metadata events are separate from release events so consumers can choose their own presentation.
7. Notification grouping uses one event per change category per scan; three new chapters become one `release.added` event with three release references, not three messages.
