package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS programs (
	id        INTEGER PRIMARY KEY,
	platform  TEXT NOT NULL,
	handle    TEXT NOT NULL,
	url       TEXT NOT NULL UNIQUE,
	last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_programs_platform ON programs(platform);
CREATE INDEX IF NOT EXISTS idx_programs_url ON programs(url);
CREATE TABLE IF NOT EXISTS targets (
	id                INTEGER PRIMARY KEY,
	program_id        INTEGER NOT NULL,
	target_normalized TEXT NOT NULL,
	target_raw        TEXT NOT NULL,
	category          TEXT NOT NULL,
	description       TEXT,
	in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
	first_seen_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY(program_id) REFERENCES programs(id),
	UNIQUE(program_id, target_raw, category)
);
CREATE INDEX IF NOT EXISTS idx_targets_program_id ON targets(program_id);
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
`

func Open(path string) (*DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
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

	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. Get or create program
	var programID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM programs WHERE url = ?", programURL).Scan(&programID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.ExecContext(ctx, "INSERT INTO programs(platform, handle, url, last_seen_at) VALUES(?,?,?,CURRENT_TIMESTAMP)", platform, handle, programURL)
		if err != nil {
			return nil, fmt.Errorf("inserting program: %w", err)
		}
		programID, err = res.LastInsertId()
		if err != nil {
			return nil, err
		}
	} else {
		// Program exists, update last_seen_at
		_, err = tx.ExecContext(ctx, "UPDATE programs SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?", programID)
		if err != nil {
			return nil, err
		}
	}

	// 2. Get existing targets for this program
	rows, err := tx.QueryContext(ctx, "SELECT id, target_raw, target_normalized, category, in_scope, description FROM targets WHERE program_id = ?", programID)
	if err != nil {
		return nil, err
	}

	type existingTarget struct {
		ID                   int64
		Raw, Norm, Cat, Desc string
		InScope              bool
	}
	existingMap := make(map[string]existingTarget)

	for rows.Next() {
		var (
			id, inScope    int64
			raw, norm, cat string
			desc           sql.NullString
		)
		if err = rows.Scan(&id, &raw, &norm, &cat, &inScope, &desc); err != nil {
			rows.Close()
			return nil, err
		}
		key := identityKey(raw, cat)
		existingMap[key] = existingTarget{ID: id, Raw: raw, Norm: norm, Cat: cat, Desc: desc.String, InScope: inScope == 1}
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}

	// 3. Insert or update new entries
	var changes []Change
	processedKeys := make(map[string]bool)

	for _, e := range entries {
		key := identityKey(e.TargetRaw, e.Category)
		if processedKeys[key] {
			continue // Skip duplicates within the same API response
		}

		inScopeInt := boolToInt(e.InScope)
		ex, existed := existingMap[key]

		if !existed {
			_, err = tx.ExecContext(ctx, `INSERT INTO targets(program_id, target_normalized, target_raw, category, description, in_scope, first_seen_at, last_seen_at) VALUES(?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
				programID, e.TargetNormalized, e.TargetRaw, e.Category, nullIfEmpty(e.Description), inScopeInt)
			if err != nil {
				return nil, err
			}
			changes = append(changes, Change{OccurredAt: now, ProgramURL: programURL, Platform: platform, Handle: handle, TargetNormalized: e.TargetNormalized, Category: e.Category, InScope: e.InScope, ChangeType: "added"})
		} else {
			if ex.Desc != e.Description || ex.InScope != e.InScope {
				_, err = tx.ExecContext(ctx, `UPDATE targets SET description = ?, in_scope = ?, last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`, nullIfEmpty(e.Description), inScopeInt, ex.ID)
				if err != nil {
					return nil, err
				}
				changes = append(changes, Change{OccurredAt: now, ProgramURL: programURL, Platform: platform, Handle: handle, TargetNormalized: e.TargetNormalized, Category: e.Category, InScope: e.InScope, ChangeType: "updated"})
			} else {
				// Just update last_seen_at
				_, err = tx.ExecContext(ctx, `UPDATE targets SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`, ex.ID)
				if err != nil {
					return nil, err
				}
			}
		}
		processedKeys[key] = true
	}

	// 4. Sweep for removed targets
	for key, ex := range existingMap {
		if !processedKeys[key] {
			// This target existed in the DB but wasn't in the latest poll, so it's removed.
			_, err = tx.ExecContext(ctx, `DELETE FROM targets WHERE id = ?`, ex.ID)
			if err != nil {
				return nil, err
			}
			_, ierr := tx.ExecContext(ctx, `INSERT INTO scope_changes(occurred_at, program_url, platform, handle, target_normalized, category, in_scope, change_type) VALUES(CURRENT_TIMESTAMP, ?, ?, ?, ?, ?, ?, 'removed')`, programURL, platform, handle, ex.Norm, ex.Cat, boolToInt(ex.InScope))
			if ierr != nil {
				return nil, ierr
			}
			changes = append(changes, Change{OccurredAt: now, ProgramURL: programURL, Platform: platform, Handle: handle, TargetNormalized: ex.Norm, Category: ex.Cat, InScope: ex.InScope, ChangeType: "removed"})
		}
	}

	return changes, tx.Commit()
}

// BuildEntries unchanged...
func BuildEntries(programURL, platform, handle string, items []TargetItem) ([]Entry, error) {
	if programURL == "" || platform == "" {
		return nil, errors.New("invalid program identifiers")
	}
	out := make([]Entry, 0, len(items))
	for _, it := range items {
		normalized := NormalizeTarget(it.URI)
		unifiedCategory := scope.CategoryUnifier(it.Category, it.URI)

		out = append(out, Entry{
			ProgramURL:       NormalizeProgramURL(programURL),
			Platform:         platform,
			Handle:           handle,
			TargetNormalized: normalized,
			TargetRaw:        it.URI,
			Category:         unifiedCategory,
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
		where += " AND p.platform = ?"
		args = append(args, opts.Platform)
	}
	if opts.ProgramFilter != "" {
		where += " AND p.url LIKE ?"
		args = append(args, fmt.Sprintf("%%%s%%", opts.ProgramFilter))
	}
	if !opts.IncludeOOS {
		where += " AND t.in_scope = 1"
	}
	if !opts.Since.IsZero() {
		where += " AND t.last_seen_at >= ?"
		args = append(args, opts.Since.UTC())
	}

	q := `
		SELECT p.url, p.platform, p.handle, t.target_normalized, t.target_raw, t.category, t.description, t.in_scope 
		FROM targets t JOIN programs p ON t.program_id = p.id 
	` + where + " ORDER BY p.url, t.target_normalized"

	rows, err := d.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var inScopeInt int
		var rawNS, descNS sql.NullString
		if err := rows.Scan(&e.ProgramURL, &e.Platform, &e.Handle, &e.TargetNormalized, &rawNS, &e.Category, &descNS, &inScopeInt); err != nil {
			return nil, err
		}
		e.TargetRaw = rawNS.String
		e.Description = descNS.String
		e.InScope = inScopeInt == 1
		out = append(out, e)
	}
	return out, rows.Err()
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
		if t, perr := time.Parse("2006-01-02 15:04:05", occurredAtStr); perr == nil {
			c.OccurredAt = t
		} else if t2, perr2 := time.Parse(time.RFC3339, occurredAtStr); perr2 == nil {
			c.OccurredAt = t2
		}
		c.InScope = inScopeInt == 1
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

type PlatformStats struct {
	Platform        string
	ProgramCount    int
	InScopeCount    int
	OutOfScopeCount int
}

func (d *DB) GetStats(ctx context.Context) ([]PlatformStats, error) {
	query := `
		SELECT
			p.platform,
			COUNT(DISTINCT p.id),
			SUM(CASE WHEN t.in_scope = 1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN t.in_scope = 0 THEN 1 ELSE 0 END)
		FROM
			programs p JOIN targets t ON p.id = t.program_id
		GROUP BY
			p.platform
		ORDER BY
			p.platform;
	`
	rows, err := d.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PlatformStats
	for rows.Next() {
		var s PlatformStats
		if err := rows.Scan(&s.Platform, &s.ProgramCount, &s.InScopeCount, &s.OutOfScopeCount); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

func (d *DB) SearchTargets(ctx context.Context, searchTerm string) ([]Entry, error) {
	likeQuery := fmt.Sprintf("%%%s%%", searchTerm)

	query := `
		SELECT p.url, p.platform, p.handle, t.target_normalized, t.target_raw, t.category, t.description, t.in_scope, 0 as is_historical
		FROM targets t 
		JOIN programs p ON t.program_id = p.id 
		WHERE t.target_normalized LIKE ? OR t.description LIKE ?

		UNION

		SELECT c.program_url, c.platform, c.handle, c.target_normalized, '' as target_raw, c.category, '' as description, c.in_scope, 1 as is_historical
		FROM scope_changes c
		WHERE c.target_normalized LIKE ?;
	`

	rows, err := d.sql.QueryContext(ctx, query, likeQuery, likeQuery, likeQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	seen := make(map[string]bool)

	for rows.Next() {
		var e Entry
		var inScopeInt, isHistoricalInt int
		var rawNS, descNS sql.NullString
		if err := rows.Scan(&e.ProgramURL, &e.Platform, &e.Handle, &e.TargetNormalized, &rawNS, &e.Category, &descNS, &inScopeInt, &isHistoricalInt); err != nil {
			return nil, err
		}
		e.TargetRaw = rawNS.String
		e.Description = descNS.String
		e.InScope = inScopeInt == 1
		e.IsHistorical = isHistoricalInt == 1

		// The UNION can return duplicates, so we'll filter them here.
		key := fmt.Sprintf("%s|%s|%s", e.ProgramURL, e.TargetNormalized, e.Category)
		if !seen[key] {
			out = append(out, e)
			seen[key] = true
		}
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func identityKey(raw, category string) string {
	if raw == "" || category == "" {
		return ""
	}
	return fmt.Sprintf("%s|%s", raw, category)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
