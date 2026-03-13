# Schema & Change Tracking

## Tables

### `programs`

Stores one row per program across all platforms.

| Column | Type | Description |
|--------|------|-------------|
| `id` | SERIAL | Primary key |
| `platform` | TEXT | Platform name (h1, bc, it, ywh, immunefi) |
| `handle` | TEXT | Platform-specific program handle |
| `url` | TEXT | Unique program URL |
| `first_seen_at` | TIMESTAMP | When the program was first polled |
| `last_seen_at` | TIMESTAMP | Last successful poll |
| `strict` | INTEGER | Whether scope changes should be treated strictly |
| `disabled` | INTEGER | 1 if program was removed from the platform |
| `is_ignored` | INTEGER | 1 if user has ignored this program |

### `targets_raw`

Raw scope entries as received from the platform.

| Column | Type | Description |
|--------|------|-------------|
| `id` | SERIAL | Primary key |
| `program_id` | INTEGER | FK to programs |
| `target` | TEXT | Raw target string |
| `category` | TEXT | Normalized category |
| `description` | TEXT | Platform-provided description |
| `in_scope` | INTEGER | 1 = in scope, 0 = out of scope |
| `is_bbp` | INTEGER | 1 = bug bounty program |
| `first_seen_at` | TIMESTAMP | When this target was first seen |
| `last_seen_at` | TIMESTAMP | Last time this target was present |

Unique constraint: `(program_id, category, target)`.

### `targets_ai_enhanced`

AI-normalized variants of raw targets. Linked to `targets_raw` rows.

### `scope_changes`

Change log — one row per detected change (addition, removal, or update).

| Column | Type | Description |
|--------|------|-------------|
| `occurred_at` | TIMESTAMP | When the change was detected |
| `program_url` | TEXT | Program URL |
| `platform` | TEXT | Platform name |
| `target_normalized` | TEXT | Normalized target |
| `target_raw` | TEXT | Raw target string |
| `target_ai_normalized` | TEXT | AI-normalized variant (if applicable) |
| `category` | TEXT | Target category |
| `in_scope` | INTEGER | Scope status |
| `change_type` | TEXT | `added`, `removed`, or `updated` |

## Change detection

When `bbscope poll --db` runs, it compares the freshly fetched scope against what's stored in the database:

- **Added**: Target exists in the poll but not in the DB.
- **Removed**: Target exists in the DB but not in the current poll.
- **Updated**: Target exists in both but properties changed (scope status, category, description).

Changes are logged to `scope_changes` and printed to stdout.

## Safety mechanisms

- **Scope wipe protection**: If an upsert would remove all targets from a program, the update is aborted (`ErrAbortingScopeWipe`). This prevents broken pollers from wiping real data.
- **Platform-level safety**: If a platform returns 0 programs but the database has >10, the entire platform sync is skipped.
- **Program sync**: Programs no longer returned by the platform are marked `disabled`, not deleted. Their historical data remains queryable.
