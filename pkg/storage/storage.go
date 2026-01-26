package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
)

var (
	// ErrAbortingScopeWipe is returned when an update would wipe all targets from a program.
	ErrAbortingScopeWipe = errors.New("aborting update to prevent wiping out all targets for a program")
)

type DB struct {
	sql *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS programs (
	id        SERIAL PRIMARY KEY,
	platform  TEXT NOT NULL,
	handle    TEXT NOT NULL,
	url       TEXT NOT NULL UNIQUE,
	first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	strict    INTEGER NOT NULL DEFAULT 0 CHECK (strict IN (0,1)),
	disabled  INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0,1)),
	is_ignored INTEGER NOT NULL DEFAULT 0 CHECK (is_ignored IN (0,1))
);
CREATE INDEX IF NOT EXISTS idx_programs_platform ON programs(platform);
CREATE INDEX IF NOT EXISTS idx_programs_url ON programs(url);
CREATE TABLE IF NOT EXISTS targets_raw (
	id                SERIAL PRIMARY KEY,
	program_id        INTEGER NOT NULL,
	target            TEXT NOT NULL,
	category          TEXT NOT NULL,
	description       TEXT,
	in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
	is_bbp            INTEGER NOT NULL DEFAULT 0 CHECK (is_bbp IN (0,1)),
	first_seen_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY(program_id) REFERENCES programs(id),
	UNIQUE(program_id, category, target)
);
CREATE INDEX IF NOT EXISTS idx_targets_raw_program_id ON targets_raw(program_id);
CREATE TABLE IF NOT EXISTS targets_ai_enhanced (
	id                   SERIAL PRIMARY KEY,
	target_id            INTEGER NOT NULL,
	target_ai_normalized TEXT NOT NULL,
	category             TEXT,
	in_scope             INTEGER,
	first_seen_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY(target_id) REFERENCES targets_raw(id) ON DELETE CASCADE,
	UNIQUE(target_id, target_ai_normalized)
);
CREATE INDEX IF NOT EXISTS idx_targets_ai_enhanced_target_id ON targets_ai_enhanced(target_id);
CREATE TABLE IF NOT EXISTS scope_changes (
	id                SERIAL PRIMARY KEY,
	occurred_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	program_url       TEXT NOT NULL,
	platform          TEXT NOT NULL,
	handle            TEXT NOT NULL,
	target_normalized TEXT NOT NULL,
	target_raw        TEXT NOT NULL DEFAULT '',
	target_ai_normalized TEXT NOT NULL DEFAULT '',
	category          TEXT NOT NULL,
	in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
	is_bbp            INTEGER NOT NULL DEFAULT 0 CHECK (is_bbp IN (0,1)),
	change_type       TEXT NOT NULL CHECK (change_type IN ('added','updated','removed'))
);
CREATE INDEX IF NOT EXISTS idx_changes_time ON scope_changes(occurred_at);
CREATE INDEX IF NOT EXISTS idx_changes_program ON scope_changes(program_url, occurred_at);
`

func Open(connectionString string) (*DB, error) {
	// connectionString is expected to be a valid Postgres connection URL or DSN
	// e.g. "postgres://user:password@localhost/dbname?sslmode=disable"
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		// Check if database doesn't exist, try to create it
		if strings.Contains(err.Error(), "does not exist") {
			if createErr := createDatabase(connectionString); createErr != nil {
				return nil, fmt.Errorf("database does not exist and failed to create: %w", createErr)
			}
			// Retry connection after creating database
			if err = db.Ping(); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrating schema: %w", err)
	}
	return &DB{sql: db}, nil
}

// createDatabase connects to the default "postgres" database and creates the target database
func createDatabase(connectionString string) error {
	parsed, err := url.Parse(connectionString)
	if err != nil {
		return fmt.Errorf("parsing connection string: %w", err)
	}

	// Extract database name from path (e.g., "/bbscope" -> "bbscope")
	dbName := strings.TrimPrefix(parsed.Path, "/")
	if dbName == "" {
		return errors.New("no database name in connection string")
	}

	// Create connection string for the default "postgres" database
	parsed.Path = "/postgres"
	adminConnStr := parsed.String()

	adminDB, err := sql.Open("postgres", adminConnStr)
	if err != nil {
		return fmt.Errorf("connecting to postgres database: %w", err)
	}
	defer adminDB.Close()

	if err := adminDB.Ping(); err != nil {
		return fmt.Errorf("pinging postgres database: %w", err)
	}

	// Create the database (identifier can't be parameterized, but we validated it came from the URL)
	_, err = adminDB.Exec(fmt.Sprintf(`CREATE DATABASE %q`, dbName))
	if err != nil {
		return fmt.Errorf("creating database %s: %w", dbName, err)
	}

	return nil
}

func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

// getOrCreateProgram handles the atomic retrieval or creation of a program entry.
func (d *DB) getOrCreateProgram(ctx context.Context, programURL, platform, handle string) (int64, error) {
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var programID int64
	row := tx.QueryRowContext(ctx, `
		INSERT INTO programs(platform, handle, url, first_seen_at, last_seen_at)
		VALUES($1,$2,$3,CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(url) DO UPDATE SET
			platform = excluded.platform,
			handle = excluded.handle,
			last_seen_at = CURRENT_TIMESTAMP,
			disabled = 0
		RETURNING id
	`, platform, handle, programURL)
	if err := row.Scan(&programID); err != nil {
		return 0, fmt.Errorf("upserting program: %w", err)
	}

	return programID, tx.Commit()
}

func (d *DB) UpsertProgramEntries(ctx context.Context, programURL, platform, handle string, entries []UpsertEntry) ([]Change, error) {
	now := time.Now().UTC()

	// 1. Get or create program
	programID, err := d.getOrCreateProgram(ctx, programURL, platform, handle)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create program: %w", err)
	}

	type existingVariant struct {
		ID          int64
		Norm        string
		Category    string
		HasCategory bool
		HasInScope  bool
		InScope     bool
	}

	type existingTarget struct {
		ID       int64
		Raw      string
		Cat      string
		Desc     string
		InScope  bool
		IsBBP    bool
		Variants map[string]existingVariant
	}

	// 2. Load existing targets for this program
	rows, err := d.sql.QueryContext(ctx, "SELECT id, target, category, in_scope, description, is_bbp FROM targets_raw WHERE program_id = $1", programID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	existingMap := make(map[string]*existingTarget)
	existingByID := make(map[int64]*existingTarget)

	for rows.Next() {
		var (
			id, inScope, isBBP int64
			raw, cat           string
			desc               sql.NullString
		)
		if err = rows.Scan(&id, &raw, &cat, &inScope, &desc, &isBBP); err != nil {
			return nil, err
		}
		key := identityKey(raw, cat)
		ex := &existingTarget{
			ID:       id,
			Raw:      raw,
			Cat:      cat,
			Desc:     desc.String,
			InScope:  inScope == 1,
			IsBBP:    isBBP == 1,
			Variants: make(map[string]existingVariant),
		}
		existingMap[key] = ex
		existingByID[id] = ex
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}

	// 3. Load existing AI enhancements tied to those targets
	variantRows, err := d.sql.QueryContext(ctx, `
		SELECT v.id, v.target_id, v.target_ai_normalized, v.category, v.in_scope
		FROM targets_ai_enhanced v
		JOIN targets_raw t ON v.target_id = t.id
		WHERE t.program_id = $1
	`, programID)
	if err != nil {
		return nil, err
	}
	defer variantRows.Close()

	for variantRows.Next() {
		var (
			id, targetID int64
			normAI       string
			catNS        sql.NullString
			inScopeNS    sql.NullInt64
		)
		if err := variantRows.Scan(&id, &targetID, &normAI, &catNS, &inScopeNS); err != nil {
			return nil, err
		}
		if target, ok := existingByID[targetID]; ok {
			if normAI != "" {
				if target.Variants == nil {
					target.Variants = make(map[string]existingVariant)
				}
				target.Variants[normAI] = existingVariant{
					ID:          id,
					Norm:        normAI,
					Category:    strings.ToLower(catNS.String),
					HasCategory: catNS.Valid,
					HasInScope:  inScopeNS.Valid,
					InScope:     inScopeNS.Int64 == 1,
				}
			}
		}
	}
	if err := variantRows.Close(); err != nil {
		return nil, err
	}

	// SAFETY CHECK
	if len(entries) == 0 && len(existingMap) > 0 {
		return nil, ErrAbortingScopeWipe
	}

	// 4. Compare incoming data against existing state
	var changes []Change
	processedKeys := make(map[string]bool)
	entryByKey := make(map[string]UpsertEntry)
	targetIDs := make(map[string]int64, len(existingMap))
	for key, ex := range existingMap {
		targetIDs[key] = ex.ID
	}

	toAdd := []UpsertEntry{}
	toUpdate := []struct {
		entry UpsertEntry
		id    int64
	}{}
	toTouch := []int64{}

	for _, e := range entries {
		key := identityKey(e.TargetRaw, e.Category)
		if key == "" {
			continue
		}
		if processedKeys[key] {
			continue
		}
		processedKeys[key] = true
		entryByKey[key] = e

		ex, existed := existingMap[key]
		if !existed {
			toAdd = append(toAdd, e)
			changes = append(changes, Change{
				OccurredAt:       now,
				ProgramURL:       programURL,
				Platform:         platform,
				Handle:           handle,
				TargetRaw:        e.TargetRaw,
				TargetNormalized: e.TargetNormalized,
				Category:         e.Category,
				InScope:          e.InScope,
				IsBBP:            e.IsBBP,
				ChangeType:       "added",
			})
		} else {
			if ex.Desc != e.Description || ex.InScope != e.InScope || ex.IsBBP != e.IsBBP {
				toUpdate = append(toUpdate, struct {
					entry UpsertEntry
					id    int64
				}{entry: e, id: ex.ID})
				changes = append(changes, Change{
					OccurredAt:       now,
					ProgramURL:       programURL,
					Platform:         platform,
					Handle:           handle,
					TargetRaw:        e.TargetRaw,
					TargetNormalized: e.TargetNormalized,
					Category:         e.Category,
					InScope:          e.InScope,
					IsBBP:            e.IsBBP,
					ChangeType:       "updated",
				})
			} else {
				toTouch = append(toTouch, ex.ID)
			}
		}
	}

	var toRemove []*existingTarget
	for key, ex := range existingMap {
		if !processedKeys[key] {
			copied := *ex
			toRemove = append(toRemove, &copied)
			normalized := NormalizeTarget(ex.Raw)
			changes = append(changes, Change{
				OccurredAt:       now,
				ProgramURL:       programURL,
				Platform:         platform,
				Handle:           handle,
				TargetRaw:        ex.Raw,
				TargetNormalized: normalized,
				Category:         ex.Cat,
				InScope:          ex.InScope,
				IsBBP:            ex.IsBBP,
				ChangeType:       "removed",
			})
		}
	}

	// 4. Start a transaction for all the batched write operations
	if len(toAdd) > 0 {
		// Prepare arrays for bulk insert using UNNEST
		targets := make([]string, len(toAdd))
		categories := make([]string, len(toAdd))
		descriptions := make([]sql.NullString, len(toAdd))
		inScopes := make([]int, len(toAdd))
		isBBPs := make([]int, len(toAdd))

		// Build a lookup map for matching returned rows
		addEntryByKey := make(map[string]UpsertEntry, len(toAdd))
		for i, e := range toAdd {
			targets[i] = e.TargetRaw
			categories[i] = e.Category
			if e.Description != "" {
				descriptions[i] = sql.NullString{String: e.Description, Valid: true}
			}
			inScopes[i] = boolToInt(e.InScope)
			isBBPs[i] = boolToInt(e.IsBBP)
			addEntryByKey[identityKey(e.TargetRaw, e.Category)] = e
		}

		// Bulk insert using UNNEST - returns id, target, category to match back
		rows, err := d.sql.QueryContext(ctx, `
			INSERT INTO targets_raw(program_id, target, category, description, in_scope, is_bbp, first_seen_at, last_seen_at)
			SELECT $1, t.target, t.category, t.description, t.in_scope, t.is_bbp, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
			FROM UNNEST($2::text[], $3::text[], $4::text[], $5::int[], $6::int[]) AS t(target, category, description, in_scope, is_bbp)
			ON CONFLICT(program_id, category, target) DO UPDATE SET
				description = excluded.description,
				in_scope = excluded.in_scope,
				is_bbp = excluded.is_bbp,
				last_seen_at = CURRENT_TIMESTAMP
			RETURNING id, target, category
		`, programID, pq.Array(targets), pq.Array(categories), pq.Array(descriptions), pq.Array(inScopes), pq.Array(isBBPs))
		if err != nil {
			return nil, fmt.Errorf("bulk inserting targets: %w", err)
		}

		for rows.Next() {
			var id int64
			var target, category string
			if err := rows.Scan(&id, &target, &category); err != nil {
				rows.Close()
				return nil, err
			}
			key := identityKey(target, category)
			targetIDs[key] = id
			if e, ok := addEntryByKey[key]; ok {
				ex := &existingTarget{
					ID:       id,
					Raw:      e.TargetRaw,
					Cat:      e.Category,
					Desc:     e.Description,
					InScope:  e.InScope,
					IsBBP:    e.IsBBP,
					Variants: make(map[string]existingVariant),
				}
				existingMap[key] = ex
			}
		}
		rows.Close()
	}

	// Batch Updates
	if len(toUpdate) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `UPDATE targets_raw SET description = $1, in_scope = $2, is_bbp = $3, last_seen_at = CURRENT_TIMESTAMP WHERE id = $4`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, u := range toUpdate {
			_, err := stmt.ExecContext(ctx, nullIfEmpty(u.entry.Description), boolToInt(u.entry.InScope), boolToInt(u.entry.IsBBP), u.id)
			if err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	// Batch Touches (update last_seen_at) - single query using ANY
	if len(toTouch) > 0 {
		_, err := d.sql.ExecContext(ctx, `UPDATE targets_raw SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ANY($1::bigint[])`, pq.Array(toTouch))
		if err != nil {
			return nil, fmt.Errorf("batch touching targets: %w", err)
		}
	}

	// Synchronize AI enhancements for remaining entries
	type variantAddOp struct {
		targetID int64
		key      string
		entry    UpsertEntry
		variant  EntryVariant
	}
	type variantUpdateOp struct {
		id      int64
		key     string
		entry   UpsertEntry
		variant EntryVariant
	}
	type variantDeleteOp struct {
		id      int64
		key     string
		entry   UpsertEntry
		variant existingVariant
	}

	var (
		variantAdds    []variantAddOp
		variantUpdates []variantUpdateOp
		variantDeletes []variantDeleteOp
	)

	for key, entry := range entryByKey {
		targetID, ok := targetIDs[key]
		if !ok || targetID == 0 {
			continue
		}
		existing := existingMap[key]
		if existing == nil {
			continue
		}
		if existing.Variants == nil {
			existing.Variants = make(map[string]existingVariant)
		}

		desired := make(map[string]EntryVariant, len(entry.Variants))
		for _, variant := range entry.Variants {
			if variant.AINormalized == "" {
				continue
			}
			desired[variant.AINormalized] = variant
		}

		for norm, variant := range desired {
			if ev, found := existing.Variants[norm]; !found {
				variantAdds = append(variantAdds, variantAddOp{
					targetID: targetID,
					key:      key,
					entry:    entry,
					variant:  variant,
				})
				changeCategory := entry.Category
				if variant.HasCategory && !strings.EqualFold(variant.Category, entry.Category) {
					changeCategory = variant.Category
				} else {
					variant.HasCategory = false
				}
				changeInScope := entry.InScope
				if variant.HasInScope {
					changeInScope = variant.InScope
				}
				changes = append(changes, Change{
					OccurredAt:         now,
					ProgramURL:         programURL,
					Platform:           platform,
					Handle:             handle,
					TargetRaw:          entry.TargetRaw,
					TargetNormalized:   entry.TargetNormalized,
					TargetAINormalized: variant.AINormalized,
					Category:           changeCategory,
					InScope:            changeInScope,
					IsBBP:              entry.IsBBP,
					ChangeType:         "added",
				})
			} else {
				needsUpdate := false
				if variant.HasInScope != ev.HasInScope {
					needsUpdate = true
				} else if variant.HasInScope && ev.InScope != variant.InScope {
					needsUpdate = true
				}
				if variant.HasCategory != ev.HasCategory {
					needsUpdate = true
				} else if variant.HasCategory && !strings.EqualFold(ev.Category, variant.Category) {
					needsUpdate = true
				}
				if !needsUpdate {
					continue
				}
				variantUpdates = append(variantUpdates, variantUpdateOp{
					id:      ev.ID,
					key:     key,
					entry:   entry,
					variant: variant,
				})
				changeCategory := entry.Category
				if variant.HasCategory && !strings.EqualFold(variant.Category, entry.Category) {
					changeCategory = variant.Category
				} else {
					variant.HasCategory = false
				}
				changeInScope := entry.InScope
				if variant.HasInScope && variant.InScope != entry.InScope {
					changeInScope = variant.InScope
				} else {
					variant.HasInScope = false
				}
				changes = append(changes, Change{
					OccurredAt:         now,
					ProgramURL:         programURL,
					Platform:           platform,
					Handle:             handle,
					TargetRaw:          entry.TargetRaw,
					TargetNormalized:   entry.TargetNormalized,
					TargetAINormalized: variant.AINormalized,
					Category:           changeCategory,
					InScope:            changeInScope,
					IsBBP:              entry.IsBBP,
					ChangeType:         "updated",
				})
			}
		}

		for norm, ev := range existing.Variants {
			if _, desiredExists := desired[norm]; desiredExists {
				continue
			}
			variantDeletes = append(variantDeletes, variantDeleteOp{
				id:      ev.ID,
				key:     key,
				entry:   entry,
				variant: ev,
			})
			changeCategory := entry.Category
			if ev.HasCategory {
				changeCategory = ev.Category
			}
			changeInScope := entry.InScope
			if ev.HasInScope {
				changeInScope = ev.InScope
			}
			changes = append(changes, Change{
				OccurredAt:         now,
				ProgramURL:         programURL,
				Platform:           platform,
				Handle:             handle,
				TargetRaw:          entry.TargetRaw,
				TargetNormalized:   entry.TargetNormalized,
				TargetAINormalized: ev.Norm,
				Category:           changeCategory,
				InScope:            changeInScope,
				IsBBP:              entry.IsBBP,
				ChangeType:         "removed",
			})
		}
	}

	if len(variantAdds) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO targets_ai_enhanced(target_id, target_ai_normalized, category, in_scope, first_seen_at, last_seen_at)
			VALUES($1,$2,$3,$4,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)
			ON CONFLICT(target_id, target_ai_normalized) DO UPDATE SET
				category = COALESCE(excluded.category, category),
				in_scope = COALESCE(excluded.in_scope, in_scope),
				last_seen_at = CURRENT_TIMESTAMP
			RETURNING id
		`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, add := range variantAdds {
			var catVal interface{}
			if add.variant.HasCategory && !strings.EqualFold(add.variant.Category, add.entry.Category) {
				catVal = add.variant.Category
			} else {
				add.variant.HasCategory = false
			}
			var inScopeVal interface{}
			if add.variant.HasInScope && add.variant.InScope != add.entry.InScope {
				inScopeVal = boolToInt(add.variant.InScope)
			} else {
				add.variant.HasInScope = false
			}
			var id int64
			if err := stmt.QueryRowContext(ctx, add.targetID, add.variant.AINormalized, catVal, inScopeVal).Scan(&id); err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
			if existing := existingMap[add.key]; existing != nil {
				existing.Variants[add.variant.AINormalized] = existingVariant{
					ID:          id,
					Norm:        add.variant.AINormalized,
					Category:    add.variant.Category,
					HasCategory: add.variant.HasCategory,
					HasInScope:  add.variant.HasInScope,
					InScope:     add.variant.InScope,
				}
			}
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	if len(variantUpdates) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `UPDATE targets_ai_enhanced SET target_ai_normalized = $1, category = $2, in_scope = $3, last_seen_at = CURRENT_TIMESTAMP WHERE id = $4`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, upd := range variantUpdates {
			var catVal interface{}
			if upd.variant.HasCategory && !strings.EqualFold(upd.variant.Category, upd.entry.Category) {
				catVal = upd.variant.Category
			} else {
				upd.variant.HasCategory = false
			}
			var inScopeVal interface{}
			if upd.variant.HasInScope && upd.variant.InScope != upd.entry.InScope {
				inScopeVal = boolToInt(upd.variant.InScope)
			} else {
				upd.variant.HasInScope = false
			}
			if _, err := stmt.ExecContext(ctx, upd.variant.AINormalized, catVal, inScopeVal, upd.id); err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
			if existing := existingMap[upd.key]; existing != nil {
				existing.Variants[upd.variant.AINormalized] = existingVariant{
					ID:          upd.id,
					Norm:        upd.variant.AINormalized,
					Category:    upd.variant.Category,
					HasCategory: upd.variant.HasCategory,
					HasInScope:  upd.variant.HasInScope,
					InScope:     upd.variant.InScope,
				}
			}
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	if len(variantDeletes) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `DELETE FROM targets_ai_enhanced WHERE id = $1`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, del := range variantDeletes {
			if _, err := stmt.ExecContext(ctx, del.id); err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
			if existing := existingMap[del.key]; existing != nil {
				delete(existing.Variants, del.variant.Norm)
			}
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	// Batch Deletes - single query using ANY
	if len(toRemove) > 0 {
		ids := make([]int64, len(toRemove))
		for i, ex := range toRemove {
			ids[i] = ex.ID
		}
		_, err := d.sql.ExecContext(ctx, `DELETE FROM targets_raw WHERE id = ANY($1::bigint[])`, pq.Array(ids))
		if err != nil {
			return nil, fmt.Errorf("batch deleting targets: %w", err)
		}
	}

	return changes, nil
}

// SetProgramIgnoredStatus sets the is_ignored flag for a program.
func (d *DB) SetProgramIgnoredStatus(ctx context.Context, programURL string, ignored bool) error {
	res, err := d.sql.ExecContext(ctx, "UPDATE programs SET is_ignored = $1 WHERE url LIKE $2", boolToInt(ignored), fmt.Sprintf("%%%s%%", programURL))
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("no program found matching URL pattern: %s", programURL)
	}
	return nil
}

// GetIgnoredPrograms returns a map of program URLs that are marked as ignored for a specific platform.
func (d *DB) GetIgnoredPrograms(ctx context.Context, platform string) (map[string]bool, error) {
	rows, err := d.sql.QueryContext(ctx, "SELECT url FROM programs WHERE platform = $1 AND is_ignored = 1", platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ignoredMap := make(map[string]bool)
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		ignoredMap[url] = true
	}
	return ignoredMap, rows.Err()
}

// GetActiveProgramCount returns the number of active (not disabled, not ignored) programs for a platform.
func (d *DB) GetActiveProgramCount(ctx context.Context, platform string) (int, error) {
	var count int
	err := d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM programs WHERE platform = $1 AND disabled = 0 AND is_ignored = 0", platform).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// SyncPlatformPrograms marks programs that are no longer returned by a platform's API as 'disabled'
// and logs their removal as a single event, preventing spam from individual target removals.
func (d *DB) SyncPlatformPrograms(ctx context.Context, platform string, polledProgramURLs []string) ([]Change, error) {
	now := time.Now().UTC()
	var changes []Change

	// 1. Create a set of polled URLs for efficient lookup.
	polledURLSet := make(map[string]struct{}, len(polledProgramURLs))
	for _, url := range polledProgramURLs {
		polledURLSet[url] = struct{}{}
	}

	// 2. Get all active programs for this platform from the DB (read operation, no transaction needed).
	rows, err := d.sql.QueryContext(ctx, `
		SELECT p.id, p.url, p.handle
		FROM programs p
		WHERE p.platform = $1 AND p.disabled = 0 AND p.is_ignored = 0
	`, platform)
	if err != nil {
		return nil, fmt.Errorf("querying for active programs: %w", err)
	}
	defer rows.Close()

	type programToRemove struct {
		ID     int64
		URL    string
		Handle string
	}
	var toRemove []programToRemove

	for rows.Next() {
		var p programToRemove
		if err := rows.Scan(&p.ID, &p.URL, &p.Handle); err != nil {
			return nil, err
		}
		if _, found := polledURLSet[p.URL]; !found {
			toRemove = append(toRemove, p)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 3. For each program that was not in the latest poll, process its removal
	// in its own short-lived transaction to avoid long-held locks.
	for _, p := range toRemove {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, fmt.Errorf("starting transaction for program removal %d: %w", p.ID, err)
		}

		// Mark the program as disabled
		if _, err := tx.ExecContext(ctx, `UPDATE programs SET disabled = 1 WHERE id = $1`, p.ID); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("disabling program %d: %w", p.ID, err)
		}

		// Delete associated targets
		if _, err := tx.ExecContext(ctx, `DELETE FROM targets_raw WHERE program_id = $1`, p.ID); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("deleting targets for program %d: %w", p.ID, err)
		}

		// Create a single "removed" change event for the entire program
		change := Change{
			OccurredAt:       now,
			ProgramURL:       p.URL,
			Platform:         platform,
			Handle:           p.Handle,
			TargetNormalized: p.URL, // Use the program URL as the "target"
			TargetRaw:        p.URL,
			Category:         "program",
			InScope:          false,
			IsBBP:            false,
			ChangeType:       "removed",
		}
		changes = append(changes, change)

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing transaction for program removal %d: %w", p.ID, err)
		}
	}

	return changes, nil
}

func (d *DB) LogChanges(ctx context.Context, changes []Change) error {
	if len(changes) == 0 {
		return nil
	}

	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO scope_changes(occurred_at, program_url, platform, handle, target_normalized, target_raw, target_ai_normalized, category, in_scope, is_bbp, change_type) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range changes {
		_, err := stmt.ExecContext(ctx, c.OccurredAt, c.ProgramURL, c.Platform, c.Handle, c.TargetNormalized, c.TargetRaw, c.TargetAINormalized, c.Category, boolToInt(c.InScope), boolToInt(c.IsBBP), c.ChangeType)
		if err != nil {
			return err // Rollback will be called
		}
	}

	return tx.Commit()
}

// BuildEntries canonicalizes raw items into upsert entries with optional variants.
func BuildEntries(programURL, platform, handle string, items []TargetItem) ([]UpsertEntry, error) {
	if programURL == "" || platform == "" {
		return nil, errors.New("invalid program identifiers")
	}
	out := make([]UpsertEntry, 0, len(items))
	for _, it := range items {
		normalized := NormalizeTarget(it.URI)
		normalizedCategory := scope.NormalizeCategory(it.Category)

		entry := UpsertEntry{
			ProgramURL:       NormalizeProgramURL(programURL),
			Platform:         platform,
			Handle:           handle,
			TargetNormalized: normalized,
			TargetRaw:        it.URI,
			Category:         normalizedCategory,
			Description:      it.Description,
			InScope:          it.InScope,
			IsBBP:            it.IsBBP,
		}

		if len(it.Variants) > 0 {
			entry.Variants = make([]EntryVariant, 0, len(it.Variants))
			for _, variant := range it.Variants {
				rawValue := strings.TrimSpace(variant.Value)
				if rawValue == "" {
					continue
				}
				variantNorm := NormalizeTarget(rawValue)
				inScope := entry.InScope
				hasInScope := false
				if variant.HasInScope {
					hasInScope = true
					inScope = variant.InScope
				}
				var cat string
				var hasCat bool
				if variant.HasCategory && scope.IsUnifiedCategory(strings.ToLower(strings.TrimSpace(variant.Category))) {
					cat = strings.ToLower(strings.TrimSpace(variant.Category))
					hasCat = true
				}
				entry.Variants = append(entry.Variants, EntryVariant{
					AINormalized: variantNorm,
					HasInScope:   hasInScope,
					InScope:      inScope,
					HasCategory:  hasCat,
					Category:     cat,
				})
			}
		}

		out = append(out, entry)
	}
	return out, nil
}

// ListAICoveredTargets returns a set of normalized target+category keys that already have AI enhancements.
func (d *DB) ListAIEnhancements(ctx context.Context, programURL string) (map[string][]TargetVariant, error) {
	result := make(map[string][]TargetVariant)
	if programURL == "" {
		return result, nil
	}

	var programID int64
	err := d.sql.QueryRowContext(ctx, "SELECT id FROM programs WHERE url = $1", programURL).Scan(&programID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := d.sql.QueryContext(ctx, `
		SELECT t.target, t.category, a.target_ai_normalized, a.in_scope, a.category
		FROM targets_ai_enhanced a
		JOIN targets_raw t ON a.target_id = t.id
		WHERE t.program_id = $1
	`, programID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			rawTarget    string
			category     string
			aiValue      string
			inScopeNS    sql.NullInt64
			aiCategoryNS sql.NullString
		)
		if err := rows.Scan(&rawTarget, &category, &aiValue, &inScopeNS, &aiCategoryNS); err != nil {
			return nil, err
		}

		key := BuildTargetCategoryKey(rawTarget, category)
		if key == "" {
			continue
		}

		variant := TargetVariant{
			Value:       aiValue,
			HasInScope:  inScopeNS.Valid,
			InScope:     inScopeNS.Int64 == 1,
			HasCategory: false,
		}
		if aiCategoryNS.Valid {
			cat := strings.ToLower(strings.TrimSpace(aiCategoryNS.String))
			if scope.IsUnifiedCategory(cat) {
				variant.HasCategory = true
				variant.Category = cat
			}
		}

		result[key] = append(result[key], variant)
	}

	return result, rows.Err()
}

// BuildTargetCategoryKey creates a normalized key for a target/category combination.
func BuildTargetCategoryKey(target, category string) string {
	normTarget := strings.ToLower(NormalizeTarget(target))
	if normTarget == "" {
		normTarget = strings.ToLower(strings.TrimSpace(target))
	}
	normCategory := scope.NormalizeCategory(category)
	return fmt.Sprintf("%s|%s", normTarget, normCategory)
}

// ListOptions controls selection when listing entries.
type ListOptions struct {
	Platform       string
	ProgramFilter  string
	Since          time.Time
	IncludeOOS     bool
	IncludeIgnored bool
}

// ListEntries returns current entries matching filters.
func (d *DB) ListEntries(ctx context.Context, opts ListOptions) ([]Entry, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if opts.Platform != "" && opts.Platform != "all" {
		where += fmt.Sprintf(" AND p.platform = $%d", argIdx)
		args = append(args, opts.Platform)
		argIdx++
	}
	if opts.ProgramFilter != "" {
		filter := fmt.Sprintf("%%%s%%", opts.ProgramFilter)
		where += fmt.Sprintf(" AND p.url LIKE $%d", argIdx)
		args = append(args, filter)
		argIdx++
	}
	if !opts.IncludeOOS {
		where += " AND COALESCE(a.in_scope, t.in_scope) = 1"
	}
	if !opts.IncludeIgnored {
		where += " AND p.is_ignored = 0"
	}
	if !opts.Since.IsZero() {
		where += fmt.Sprintf(" AND COALESCE(a.last_seen_at, t.last_seen_at) >= $%d", argIdx)
		args = append(args, opts.Since.UTC())
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT 
			p.url,
			p.platform,
			p.handle,
			t.target,
			t.category,
			t.description,
			t.in_scope,
			t.is_bbp,
			a.target_ai_normalized,
			a.category,
			a.in_scope,
			a.id
		FROM targets_raw t
		JOIN programs p ON t.program_id = p.id
		LEFT JOIN targets_ai_enhanced a ON a.target_id = t.id
		%s
		ORDER BY p.url, COALESCE(a.target_ai_normalized, t.target)
	`, where)

	rows, err := d.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var (
			programURL   string
			platform     string
			handle       string
			rawTarget    string
			baseCategory string
			descNS       sql.NullString
			baseInScope  int
			isBBPInt     int
			aiTargetNS   sql.NullString
			aiCategoryNS sql.NullString
			aiInScopeNS  sql.NullInt64
			aiIDNS       sql.NullInt64
		)
		if err := rows.Scan(
			&programURL,
			&platform,
			&handle,
			&rawTarget,
			&baseCategory,
			&descNS,
			&baseInScope,
			&isBBPInt,
			&aiTargetNS,
			&aiCategoryNS,
			&aiInScopeNS,
			&aiIDNS,
		); err != nil {
			return nil, err
		}

		baseNorm := NormalizeTarget(rawTarget)
		entry := Entry{
			ProgramURL:           programURL,
			Platform:             platform,
			Handle:               handle,
			BaseTargetRaw:        rawTarget,
			BaseTargetNormalized: baseNorm,
			TargetRaw:            rawTarget,
			Description:          descNS.String,
			IsBBP:                isBBPInt == 1,
			Category:             baseCategory,
			Source:               "raw",
		}

		if aiIDNS.Valid {
			entry.Source = "ai"
			if aiTargetNS.Valid && aiTargetNS.String != "" {
				entry.TargetNormalized = aiTargetNS.String
			} else {
				entry.TargetNormalized = baseNorm
			}
			if aiCategoryNS.Valid {
				cat := strings.ToLower(strings.TrimSpace(aiCategoryNS.String))
				if scope.IsUnifiedCategory(cat) && !strings.EqualFold(cat, baseCategory) {
					entry.Category = cat
				}
			}
			if aiInScopeNS.Valid {
				entry.InScope = aiInScopeNS.Int64 == 1
			} else {
				entry.InScope = baseInScope == 1
			}
		} else {
			entry.TargetNormalized = baseNorm
			entry.InScope = baseInScope == 1
		}

		out = append(out, entry)
	}
	return out, rows.Err()
}

// AddCustomTarget adds a single target for a custom program.
// It returns true if the target was newly created, false if it already existed.
func (d *DB) AddCustomTarget(ctx context.Context, target, category, programURL string) (bool, error) {
	platform := "custom"
	// If programURL is "custom", use "custom" as the program URL (don't append target)
	if programURL == "custom" {
		programURL = "custom"
	}
	handle := target

	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var programID int64
	programRow := tx.QueryRowContext(ctx, `
		INSERT INTO programs(platform, handle, url, first_seen_at, last_seen_at)
		VALUES($1,$2,$3,CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(url) DO UPDATE SET
			platform = excluded.platform,
			handle = excluded.handle,
			last_seen_at = CURRENT_TIMESTAMP,
			disabled = 0
		RETURNING id
	`, platform, handle, programURL)
	if err := programRow.Scan(&programID); err != nil {
		return false, fmt.Errorf("upserting custom program: %w", err)
	}

	targetExists := false
	var exists int
	err = tx.QueryRowContext(ctx, `
		SELECT 1 FROM targets_raw WHERE program_id = $1 AND category = $2 AND target = $3 LIMIT 1
	`, programID, category, target).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("checking existing custom target: %w", err)
	}
	if err == nil {
		targetExists = true
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO targets_raw(program_id, target, category, in_scope, is_bbp, first_seen_at, last_seen_at)
		VALUES($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(program_id, category, target) DO UPDATE SET
			last_seen_at = CURRENT_TIMESTAMP
	`, programID, target, category, boolToInt(true), boolToInt(false))
	if err != nil {
		return false, fmt.Errorf("upserting custom target: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return !targetExists, nil
}

// ListRecentChanges returns the most recent N changes across all programs.
func (d *DB) ListRecentChanges(ctx context.Context, limit int) ([]Change, error) {
	if limit <= 0 {
		limit = 50
	}
	q := "SELECT occurred_at, program_url, platform, handle, target_normalized, target_raw, target_ai_normalized, category, in_scope, is_bbp, change_type FROM scope_changes ORDER BY occurred_at DESC LIMIT $1"
	rows, err := d.sql.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	changes := []Change{}
	for rows.Next() {
		var c Change
		var inScopeInt, isBBPInt int
		if err := rows.Scan(&c.OccurredAt, &c.ProgramURL, &c.Platform, &c.Handle, &c.TargetNormalized, &c.TargetRaw, &c.TargetAINormalized, &c.Category, &inScopeInt, &isBBPInt, &c.ChangeType); err != nil {
			return nil, fmt.Errorf("scanning change row: %w", err)
		}
		c.InScope = inScopeInt == 1
		c.IsBBP = isBBPInt == 1
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
		WITH effective_targets AS (
			SELECT t.program_id, COALESCE(a.in_scope, t.in_scope) AS in_scope
			FROM targets_raw t
			LEFT JOIN targets_ai_enhanced a ON a.target_id = t.id
		)
		SELECT
			p.platform,
			COUNT(DISTINCT p.id),
			SUM(CASE WHEN et.in_scope = 1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN et.in_scope = 0 THEN 1 ELSE 0 END)
		FROM
			programs p JOIN effective_targets et ON p.id = et.program_id
		WHERE
			p.is_ignored = 0
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
		SELECT 
			p.url,
			p.platform,
			p.handle,
			t.target,
			t.category,
			t.description,
			t.in_scope,
			t.is_bbp,
			a.target_ai_normalized,
			a.category,
			a.in_scope,
			a.id,
			'current' AS source
		FROM targets_raw t
		JOIN programs p ON t.program_id = p.id
		LEFT JOIN targets_ai_enhanced a ON a.target_id = t.id
		WHERE p.is_ignored = 0 AND (
			COALESCE(a.target_ai_normalized, t.target) LIKE $1 OR
			t.description LIKE $2 OR
			p.url LIKE $3
		)

		UNION

		SELECT 
			c.program_url,
			c.platform,
			c.handle,
			c.target_raw,
			c.category,
			NULL as description,
			CASE WHEN c.in_scope = 1 THEN 1 ELSE 0 END as in_scope,
			CASE WHEN c.is_bbp = 1 THEN 1 ELSE 0 END as is_bbp,
			c.target_ai_normalized,
			c.category,
			CASE WHEN c.in_scope = 1 THEN 1 ELSE 0 END as ai_in_scope,
			NULL as ai_id,
			'historical' as source
		FROM scope_changes c
		WHERE (c.target_normalized LIKE $4 OR c.target_ai_normalized LIKE $5 OR c.program_url LIKE $6)
		AND NOT EXISTS (
			SELECT 1 FROM targets_raw t2
			JOIN programs p2 ON t2.program_id = p2.id
			WHERE p2.url = c.program_url
			AND p2.is_ignored = 0
			AND t2.target = c.target_raw
			AND t2.category = c.category
		);
	`

	rows, err := d.sql.QueryContext(ctx, query,
		likeQuery, likeQuery, likeQuery,
		likeQuery, likeQuery, likeQuery,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	seen := make(map[string]int)

	for rows.Next() {
		var (
			programURL   string
			platform     string
			handle       string
			rawTarget    string
			baseCategory string
			descNS       sql.NullString
			baseInScope  int
			isBBPInt     int
			aiTargetNS   sql.NullString
			aiCategoryNS sql.NullString
			aiInScopeNS  sql.NullInt64
			aiIDNS       sql.NullInt64
			source       string
		)
		if err := rows.Scan(
			&programURL,
			&platform,
			&handle,
			&rawTarget,
			&baseCategory,
			&descNS,
			&baseInScope,
			&isBBPInt,
			&aiTargetNS,
			&aiCategoryNS,
			&aiInScopeNS,
			&aiIDNS,
			&source,
		); err != nil {
			return nil, err
		}

		baseNorm := NormalizeTarget(rawTarget)
		entry := Entry{
			ProgramURL:           programURL,
			Platform:             platform,
			Handle:               handle,
			Category:             baseCategory,
			Description:          descNS.String,
			BaseTargetRaw:        rawTarget,
			BaseTargetNormalized: baseNorm,
			TargetRaw:            rawTarget,
			IsBBP:                isBBPInt == 1,
		}

		if source == "historical" {
			if aiTargetNS.Valid && aiTargetNS.String != "" {
				entry.TargetNormalized = aiTargetNS.String
			} else {
				entry.TargetNormalized = baseNorm
			}
			if aiCategoryNS.Valid && scope.IsUnifiedCategory(strings.ToLower(strings.TrimSpace(aiCategoryNS.String))) {
				entry.Category = strings.ToLower(strings.TrimSpace(aiCategoryNS.String))
			}
			entry.InScope = baseInScope == 1
			entry.IsHistorical = true
		} else {
			entry.IsHistorical = false
			if aiIDNS.Valid {
				if aiTargetNS.Valid && aiTargetNS.String != "" {
					entry.TargetNormalized = aiTargetNS.String
				} else {
					entry.TargetNormalized = baseNorm
				}
				if aiCategoryNS.Valid {
					cat := strings.ToLower(strings.TrimSpace(aiCategoryNS.String))
					if scope.IsUnifiedCategory(cat) && !strings.EqualFold(cat, baseCategory) {
						entry.Category = cat
					}
				}
				if aiInScopeNS.Valid {
					entry.InScope = aiInScopeNS.Int64 == 1
				} else {
					entry.InScope = baseInScope == 1
				}
			} else {
				entry.TargetNormalized = baseNorm
				entry.InScope = baseInScope == 1
			}
		}

		if source == "historical" {
			entry.Source = "historical"
		} else if aiIDNS.Valid {
			entry.Source = "ai"
		} else {
			entry.Source = "raw"
		}

		key := fmt.Sprintf("%s|%s|%s|%s", entry.ProgramURL, entry.TargetNormalized, entry.BaseTargetNormalized, entry.Category)
		if idx, ok := seen[key]; ok {
			// Always prefer current entries over historical ones
			if out[idx].IsHistorical && !entry.IsHistorical {
				out[idx] = entry
			}
			// If we already have a current entry, skip adding a historical one
		} else {
			out = append(out, entry)
			seen[key] = len(out) - 1
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
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	category = strings.TrimSpace(strings.ToLower(category))
	return raw + "|" + category
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
