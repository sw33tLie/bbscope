# Storage Package

```go
import "github.com/sw33tLie/bbscope/v2/pkg/storage"
```

The `storage` package provides the PostgreSQL persistence layer. It handles schema creation, scope upserts with change detection, querying, and searching.

## Opening a connection

```go
db, err := storage.Open("postgres://user:pass@localhost/bbscope?sslmode=disable")
if err != nil {
    log.Fatal(err)
}
defer db.Close()
```

`Open` automatically creates all tables and indexes if they don't exist.

## Key types

### TargetItem

Input type for building entries:

```go
type TargetItem struct {
    URI         string
    Category    string
    Description string
    InScope     bool
    IsBBP       bool
    Variants    []TargetVariant  // AI-normalized variants
}
```

### Change

Represents a detected scope change:

```go
type Change struct {
    OccurredAt         time.Time
    ProgramURL         string
    Platform           string
    Handle             string
    TargetNormalized   string
    TargetRaw          string
    TargetAINormalized string
    Category           string
    InScope            bool
    IsBBP              bool
    ChangeType         string  // "added", "removed", "updated"
}
```

## Core operations

### Upserting scope data

```go
// Build entries from target items
entries, err := storage.BuildEntries(programURL, platform, handle, items)

// Upsert and get changes
changes, err := db.UpsertProgramEntries(ctx, programURL, platform, handle, entries)
```

`UpsertProgramEntries` compares the new entries against what's in the database and returns the diff as `[]Change`. It also updates timestamps and handles additions/removals/updates atomically.

### Syncing removed programs

```go
changes, err := db.SyncPlatformPrograms(ctx, "h1", polledProgramURLs)
```

Marks programs not in `polledProgramURLs` as disabled and returns removal changes.

### Logging changes

```go
err := db.LogChanges(ctx, changes)
```

Persists changes to the `scope_changes` table for later querying.

### Querying

```go
// List entries with filters
entries, err := db.ListEntries(ctx, storage.ListOptions{
    Platform: "h1",
    InScope:  true,
})

// Full-text search
results, err := db.SearchTargets(ctx, "example.com")

// Recent changes
changes, err := db.ListRecentChanges(ctx, 50)

// Statistics
stats, err := db.GetStats(ctx)
```

### Program management

```go
// Get program count
count, err := db.GetActiveProgramCount(ctx, "h1")

// Get ignored programs
ignored, err := db.GetIgnoredPrograms(ctx, "h1")

// AI enhancements cache
enhancements, err := db.ListAIEnhancements(ctx, programURL)
```

## Normalization helpers

```go
// Normalize a target string (lowercase, strip ports, etc.)
normalized := storage.NormalizeTarget(raw, category)

// Build cache key for AI enhancement lookups
key := storage.BuildTargetCategoryKey(target, category)

// Aggressive transform: extract root domain via publicsuffix
root := storage.AggressiveTransform(target)
```
