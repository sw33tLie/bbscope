package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	// Ensure schema exists for convenience.
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS scope_entries (
  id                INTEGER PRIMARY KEY,
  program_url       TEXT NOT NULL,
  platform          TEXT NOT NULL,
  handle            TEXT NOT NULL,
  target_normalized TEXT NOT NULL,
  target_raw        TEXT,
  category          TEXT NOT NULL,
  description       TEXT,
  in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
  run_id            INTEGER NOT NULL DEFAULT 0,
  first_seen_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_seen_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(program_url, target_normalized, category, in_scope)
);
CREATE INDEX IF NOT EXISTS idx_scope_program ON scope_entries(program_url);
CREATE INDEX IF NOT EXISTS idx_scope_identity ON scope_entries(program_url, target_normalized, category, in_scope);
CREATE TABLE IF NOT EXISTS scope_changes (
  id                INTEGER PRIMARY KEY,
  occurred_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  program_url       TEXT NOT NULL,
  platform          TEXT NOT NULL,
  handle            TEXT NOT NULL,
  target_normalized TEXT NOT NULL,
  category          TEXT NOT NULL,
  in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
  change_type       TEXT NOT NULL CHECK (change_type IN ('added','updated','removed'))
);
CREATE INDEX IF NOT EXISTS idx_changes_time ON scope_changes(occurred_at);
CREATE INDEX IF NOT EXISTS idx_changes_program ON scope_changes(program_url, occurred_at);
    `); err != nil {
		return nil, err
	}
	return &DB{sql: db}, nil
}

func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

func (d *DB) UpsertProgramEntries(ctx context.Context, programURL, platform, handle string, entries []Entry) ([]Change, error) {
	now := time.Now().UTC()
	runID := time.Now().Unix()

	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(ctx, "SELECT target_normalized, category, in_scope, target_raw, description FROM scope_entries WHERE program_url = ?", programURL)
	if err != nil {
		return nil, err
	}

	type existing struct{ Raw, Desc string }
	existingMap := make(map[string]existing)
	for rows.Next() {
		var (
			tn, cat   string
			inScope   int
			raw, desc sql.NullString
		)
		if err = rows.Scan(&tn, &cat, &inScope, &raw, &desc); err != nil {
			rows.Close()
			return nil, err
		}
		existingMap[identityKey(tn, cat, inScope == 1)] = existing{Raw: raw.String, Desc: desc.String}
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}

	var changes []Change
	for _, e := range entries {
		key := identityKey(e.TargetNormalized, e.Category, e.InScope)

		ex, existed := existingMap[key]
		inScopeInt := boolToInt(e.InScope)

		if !existed {
			_, err = tx.ExecContext(ctx, `INSERT INTO scope_entries(program_url, platform, handle, target_normalized, target_raw, category, description, in_scope, run_id, first_seen_at, last_seen_at) VALUES(?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, e.ProgramURL, e.Platform, e.Handle, e.TargetNormalized, nullIfEmpty(e.TargetRaw), e.Category, nullIfEmpty(e.Description), inScopeInt, runID)
			if err != nil {
				return nil, err
			}
			changes = append(changes, Change{OccurredAt: now, ProgramURL: programURL, Platform: platform, Handle: handle, TargetNormalized: e.TargetNormalized, Category: e.Category, InScope: e.InScope, ChangeType: "added"})
			existingMap[key] = existing{Raw: e.TargetRaw, Desc: e.Description} // Track the new entry
		} else {
			if ex.Raw != e.TargetRaw || ex.Desc != e.Description {
				_, err = tx.ExecContext(ctx, `UPDATE scope_entries SET target_raw = ?, description = ?, run_id = ?, last_seen_at = CURRENT_TIMESTAMP WHERE program_url = ? AND target_normalized = ? AND category = ? AND in_scope = ?`, nullIfEmpty(e.TargetRaw), nullIfEmpty(e.Description), runID, e.ProgramURL, e.TargetNormalized, e.Category, inScopeInt)
				if err != nil {
					return nil, err
				}
				changes = append(changes, Change{OccurredAt: now, ProgramURL: programURL, Platform: platform, Handle: handle, TargetNormalized: e.TargetNormalized, Category: e.Category, InScope: e.InScope, ChangeType: "updated"})
			} else {
				_, err = tx.ExecContext(ctx, `UPDATE scope_entries SET run_id = ?, last_seen_at = CURRENT_TIMESTAMP WHERE program_url = ? AND target_normalized = ? AND category = ? AND in_scope = ?`, runID, e.ProgramURL, e.TargetNormalized, e.Category, inScopeInt)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	// Sweep: find and delete entries not touched in this run, log removals
	staleRows, err := tx.QueryContext(ctx, "SELECT handle, target_normalized, category, in_scope FROM scope_entries WHERE platform = ? AND program_url = ? AND run_id != ?", platform, programURL, runID)
	if err != nil {
		return nil, err
	}

	type staleEntry struct {
		Handle, T, C string
		InScope      int
	}
	var toRemove []staleEntry
	for staleRows.Next() {
		var s staleEntry
		if err = staleRows.Scan(&s.Handle, &s.T, &s.C, &s.InScope); err != nil {
			staleRows.Close()
			return nil, err
		}
		toRemove = append(toRemove, s)
	}
	if err = staleRows.Close(); err != nil {
		return nil, err
	}

	if len(toRemove) > 0 {
		_, err = tx.ExecContext(ctx, `DELETE FROM scope_entries WHERE platform = ? AND program_url = ? AND run_id != ?`, platform, programURL, runID)
		if err != nil {
			return nil, err
		}
		for _, s := range toRemove {
			_, ierr := tx.ExecContext(ctx, `INSERT INTO scope_changes(occurred_at, program_url, platform, handle, target_normalized, category, in_scope, change_type) VALUES(CURRENT_TIMESTAMP, ?, ?, ?, ?, ?, ?, 'removed')`, programURL, platform, s.Handle, s.T, s.C, s.InScope)
			if ierr != nil {
				return nil, ierr
			}
			changes = append(changes, Change{OccurredAt: now, ProgramURL: programURL, Platform: platform, Handle: s.Handle, TargetNormalized: s.T, Category: s.C, InScope: s.InScope == 1, ChangeType: "removed"})
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return changes, nil
}

// BuildEntries unchanged
func BuildEntries(programURL, platform, handle string, items []TargetItem) ([]Entry, error) {
	if programURL == "" || platform == "" {
		return nil, errors.New("invalid program identifiers")
	}
	out := make([]Entry, 0, len(items))
	for _, it := range items {
		normalized := NormalizeTarget(it.URI)
		out = append(out, Entry{
			ProgramURL:       NormalizeProgramURL(programURL),
			Platform:         platform,
			Handle:           handle,
			TargetNormalized: normalized,
			TargetRaw:        it.URI,
			Category:         it.Category,
			Description:      it.Description,
			InScope:          it.InScope,
		})
	}
	return out, nil
}

// ListOptions controls selection when listing entries.
type ListOptions struct {
	Platform      string
	ProgramFilter string
	Since         time.Time
	IncludeOOS    bool
}

// ListEntries returns current entries matching filters.
func (d *DB) ListEntries(ctx context.Context, opts ListOptions) ([]Entry, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	if opts.Platform != "" && opts.Platform != "all" {
		where += " AND platform = ?"
		args = append(args, opts.Platform)
	}
	if opts.ProgramFilter != "" {
		where += " AND program_url LIKE ?"
		args = append(args, fmt.Sprintf("%%%s%%", opts.ProgramFilter))
	}
	if !opts.IncludeOOS {
		where += " AND in_scope = 1"
	}
	if !opts.Since.IsZero() {
		where += " AND last_seen_at >= ?"
		args = append(args, opts.Since.UTC())
	}

	q := "SELECT program_url, platform, handle, target_normalized, target_raw, category, description, in_scope FROM scope_entries " + where + " ORDER BY program_url, target_normalized"
	rows, err := d.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var inScopeInt int
		var rawNS sql.NullString
		var descNS sql.NullString
		if err := rows.Scan(&e.ProgramURL, &e.Platform, &e.Handle, &e.TargetNormalized, &rawNS, &e.Category, &descNS, &inScopeInt); err != nil {
			return nil, err
		}
		if rawNS.Valid {
			e.TargetRaw = rawNS.String
		} else {
			e.TargetRaw = ""
		}
		if descNS.Valid {
			e.Description = descNS.String
		} else {
			e.Description = ""
		}
		e.InScope = inScopeInt == 1
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListRecentChanges returns the most recent N changes across all programs.
func (d *DB) ListRecentChanges(ctx context.Context, limit int) ([]Change, error) {
	if limit <= 0 {
		limit = 50
	}
	q := "SELECT occurred_at, program_url, platform, handle, target_normalized, category, in_scope, change_type FROM scope_changes ORDER BY occurred_at DESC LIMIT ?"
	rows, err := d.sql.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	changes := []Change{}
	for rows.Next() {
		var c Change
		var occurredAtStr string
		var inScopeInt int
		if err := rows.Scan(&occurredAtStr, &c.ProgramURL, &c.Platform, &c.Handle, &c.TargetNormalized, &c.Category, &inScopeInt, &c.ChangeType); err != nil {
			return nil, err
		}
		// Parse SQLite CURRENT_TIMESTAMP format
		// Try "2006-01-02 15:04:05" then RFC3339
		if t, perr := time.Parse("2006-01-02 15:04:05", occurredAtStr); perr == nil {
			c.OccurredAt = t
		} else if t2, perr2 := time.Parse(time.RFC3339, occurredAtStr); perr2 == nil {
			c.OccurredAt = t2
		} else {
			c.OccurredAt = time.Time{}
		}
		c.InScope = inScopeInt == 1
		changes = append(changes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return changes, nil
}

type PlatformStats struct {
	Platform     string
	ProgramCount int
	TargetCount  int
}

func (d *DB) GetStats(ctx context.Context) ([]PlatformStats, error) {
	query := `
		SELECT
			platform,
			COUNT(DISTINCT program_url),
			COUNT(target_normalized)
		FROM
			scope_entries
		GROUP BY
			platform
		ORDER BY
			platform;
	`
	rows, err := d.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PlatformStats
	for rows.Next() {
		var s PlatformStats
		if err := rows.Scan(&s.Platform, &s.ProgramCount, &s.TargetCount); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
