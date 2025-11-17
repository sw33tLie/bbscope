package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	_ "modernc.org/sqlite"
)

var (
	// ErrAbortingScopeWipe is returned when an update would wipe all targets from a program.
	ErrAbortingScopeWipe = errors.New("aborting update to prevent wiping out all targets for a program")
)

const (
	// DefaultDBTimeout is the default timeout in milliseconds to wait for DB lock to be released.
	DefaultDBTimeout = 15000
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
	first_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	strict    INTEGER NOT NULL DEFAULT 0 CHECK (strict IN (0,1)),
	disabled  INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0,1)),
	is_ignored INTEGER NOT NULL DEFAULT 0 CHECK (is_ignored IN (0,1))
);
CREATE INDEX IF NOT EXISTS idx_programs_platform ON programs(platform);
CREATE INDEX IF NOT EXISTS idx_programs_url ON programs(url);
CREATE TABLE IF NOT EXISTS targets_raw (
	id                INTEGER PRIMARY KEY,
	program_id        INTEGER NOT NULL,
	target            TEXT NOT NULL,
	category          TEXT NOT NULL,
	description       TEXT,
	in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
	is_bbp            INTEGER NOT NULL DEFAULT 0 CHECK (is_bbp IN (0,1)),
	first_seen_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY(program_id) REFERENCES programs(id),
	UNIQUE(program_id, category, target)
);
CREATE INDEX IF NOT EXISTS idx_targets_raw_program_id ON targets_raw(program_id);
CREATE TABLE IF NOT EXISTS targets_expanded (
	id                 INTEGER PRIMARY KEY,
	target_id          INTEGER NOT NULL,
	target_normalized TEXT NOT NULL DEFAULT '',
	target_ai_normalized TEXT NOT NULL DEFAULT '',
	variant_raw        TEXT NOT NULL,
	in_scope           INTEGER NOT NULL CHECK (in_scope IN (0,1)),
	first_seen_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY(target_id) REFERENCES targets_raw(id) ON DELETE CASCADE,
	UNIQUE(target_id, target_normalized, target_ai_normalized, variant_raw)
);
CREATE INDEX IF NOT EXISTS idx_targets_expanded_target_id ON targets_expanded(target_id);
CREATE TABLE IF NOT EXISTS scope_changes (
	id                INTEGER PRIMARY KEY,
	occurred_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	program_url       TEXT NOT NULL,
	platform          TEXT NOT NULL,
	handle            TEXT NOT NULL,
	target_normalized TEXT NOT NULL,
	target_raw        TEXT NOT NULL DEFAULT '',
	target_ai_normalized TEXT NOT NULL DEFAULT '',
	variant_raw        TEXT NOT NULL DEFAULT '',
	category          TEXT NOT NULL,
	in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
	is_bbp            INTEGER NOT NULL DEFAULT 0 CHECK (is_bbp IN (0,1)),
	change_type       TEXT NOT NULL CHECK (change_type IN ('added','updated','removed'))
);
CREATE INDEX IF NOT EXISTS idx_changes_time ON scope_changes(occurred_at);
CREATE INDEX IF NOT EXISTS idx_changes_program ON scope_changes(program_url, occurred_at);
`

func Open(path string, timeout int) (*DB, error) {
	if timeout <= 0 {
		timeout = 5000 // Default to 5 seconds if not specified or invalid
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)", path, timeout)
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

// getOrCreateProgram handles the atomic retrieval or creation of a program entry.
// It uses a short-lived transaction to prevent race conditions and minimize lock time.
func (d *DB) getOrCreateProgram(ctx context.Context, programURL, platform, handle string) (int64, error) {
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var programID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM programs WHERE url = ?", programURL).Scan(&programID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err // A real error occurred.
	}

	if errors.Is(err, sql.ErrNoRows) {
		// Program doesn't exist, create it.
		res, err := tx.ExecContext(ctx, "INSERT INTO programs(platform, handle, url, first_seen_at, last_seen_at) VALUES(?,?,?,CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", platform, handle, programURL)
		if err != nil {
			return 0, fmt.Errorf("inserting program: %w", err)
		}
		programID, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else {
		// Program exists, update last_seen_at.
		_, err = tx.ExecContext(ctx, "UPDATE programs SET last_seen_at = CURRENT_TIMESTAMP, disabled = 0 WHERE id = ?", programID)
		if err != nil {
			return 0, err
		}
	}

	return programID, tx.Commit()
}

func (d *DB) UpsertProgramEntries(ctx context.Context, programURL, platform, handle string, entries []UpsertEntry) ([]Change, error) {
	now := time.Now().UTC()

	// 1. Get or create program in its own short transaction to minimize lock time.
	programID, err := d.getOrCreateProgram(ctx, programURL, platform, handle)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create program: %w", err)
	}

	type existingVariant struct {
		ID      int64
		Raw     string
		Norm    string
		InScope bool
	}

	type existingTarget struct {
		ID          int64
		Raw         string
		Cat         string
		Desc        string
		InScope     bool
		IsBBP       bool
		NormID      int64
		Norm        string
		NormInScope bool
		Variants    map[string]existingVariant
	}

	// 2. Load existing targets for this program
	rows, err := d.sql.QueryContext(ctx, "SELECT id, target, category, in_scope, description, is_bbp FROM targets_raw WHERE program_id = ?", programID)
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

	// 3. Load existing expansions tied to those targets
	variantRows, err := d.sql.QueryContext(ctx, `
		SELECT v.id, v.target_id, v.variant_raw, v.target_normalized, v.target_ai_normalized, v.in_scope
		FROM targets_expanded v
		JOIN targets_raw t ON v.target_id = t.id
		WHERE t.program_id = ?
	`, programID)
	if err != nil {
		return nil, err
	}
	defer variantRows.Close()

	for variantRows.Next() {
		var (
			id, targetID, inScope int64
			raw, normDet, normAI  string
		)
		if err := variantRows.Scan(&id, &targetID, &raw, &normDet, &normAI, &inScope); err != nil {
			return nil, err
		}
		if target, ok := existingByID[targetID]; ok {
			if normDet != "" {
				target.Norm = normDet
				target.NormID = id
				target.NormInScope = inScope == 1
			} else if normAI != "" {
				if target.Variants == nil {
					target.Variants = make(map[string]existingVariant)
				}
				target.Variants[normAI] = existingVariant{
					ID:      id,
					Raw:     raw,
					Norm:    normAI,
					InScope: inScope == 1,
				}
			}
		}
	}
	if err := variantRows.Close(); err != nil {
		return nil, err
	}

	// SAFETY CHECK: If the incoming data has zero entries, but we know in the DB
	// that this program HAS entries, we abort. This prevents a broken poller from wiping a scope.
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
			changes = append(changes, Change{
				OccurredAt:       now,
				ProgramURL:       programURL,
				Platform:         platform,
				Handle:           handle,
				TargetRaw:        ex.Raw,
				TargetNormalized: ex.Norm,
				Category:         ex.Cat,
				InScope:          ex.InScope,
				IsBBP:            ex.IsBBP,
				ChangeType:       "removed",
			})
		}
	}

	// 4. Start a transaction for all the batched write operations
	if len(toAdd) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO targets_raw(program_id, target, category, description, in_scope, is_bbp, first_seen_at, last_seen_at) VALUES(?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, e := range toAdd {
			res, err := stmt.ExecContext(ctx, programID, e.TargetRaw, e.Category, nullIfEmpty(e.Description), boolToInt(e.InScope), boolToInt(e.IsBBP))
			if err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
			id, err := res.LastInsertId()
			if err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
			key := identityKey(e.TargetRaw, e.Category)
			targetIDs[key] = id
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
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	// Batch Updates
	if len(toUpdate) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `UPDATE targets_raw SET description = ?, in_scope = ?, is_bbp = ?, last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`)
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

	// Batch Touches (update last_seen_at)
	if len(toTouch) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `UPDATE targets_raw SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, id := range toTouch {
			_, err := stmt.ExecContext(ctx, id)
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

	// Synchronize expansions (deterministic + AI) for remaining entries
	type detAddOp struct {
		targetID   int64
		key        string
		entry      UpsertEntry
		normalized string
		inScope    bool
	}
	type detUpdateOp struct {
		id         int64
		key        string
		entry      UpsertEntry
		normalized string
		inScope    bool
	}
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
		detAdds        []detAddOp
		detUpdates     []detUpdateOp
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

		if entry.TargetNormalized != "" {
			if existing.Norm == "" {
				detAdds = append(detAdds, detAddOp{
					targetID:   targetID,
					key:        key,
					entry:      entry,
					normalized: entry.TargetNormalized,
					inScope:    entry.InScope,
				})
			} else if existing.Norm != entry.TargetNormalized || existing.NormInScope != entry.InScope {
				detUpdates = append(detUpdates, detUpdateOp{
					id:         existing.NormID,
					key:        key,
					entry:      entry,
					normalized: entry.TargetNormalized,
					inScope:    entry.InScope,
				})
			}
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
				changes = append(changes, Change{
					OccurredAt:         now,
					ProgramURL:         programURL,
					Platform:           platform,
					Handle:             handle,
					TargetRaw:          entry.TargetRaw,
					TargetNormalized:   entry.TargetNormalized,
					VariantRaw:         variant.Raw,
					TargetAINormalized: variant.AINormalized,
					Category:           entry.Category,
					InScope:            variant.InScope,
					IsBBP:              entry.IsBBP,
					ChangeType:         "added",
				})
			} else if ev.Raw != variant.Raw || ev.InScope != variant.InScope {
				variantUpdates = append(variantUpdates, variantUpdateOp{
					id:      ev.ID,
					key:     key,
					entry:   entry,
					variant: variant,
				})
				changes = append(changes, Change{
					OccurredAt:         now,
					ProgramURL:         programURL,
					Platform:           platform,
					Handle:             handle,
					TargetRaw:          entry.TargetRaw,
					TargetNormalized:   entry.TargetNormalized,
					VariantRaw:         variant.Raw,
					TargetAINormalized: variant.AINormalized,
					Category:           entry.Category,
					InScope:            variant.InScope,
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
			changes = append(changes, Change{
				OccurredAt:         now,
				ProgramURL:         programURL,
				Platform:           platform,
				Handle:             handle,
				TargetRaw:          entry.TargetRaw,
				TargetNormalized:   entry.TargetNormalized,
				VariantRaw:         ev.Raw,
				TargetAINormalized: ev.Norm,
				Category:           entry.Category,
				InScope:            ev.InScope,
				IsBBP:              entry.IsBBP,
				ChangeType:         "removed",
			})
		}
	}

	if len(detAdds) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO targets_expanded(target_id, target_normalized, target_ai_normalized, variant_raw, in_scope, first_seen_at, last_seen_at) VALUES(?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, add := range detAdds {
			res, err := stmt.ExecContext(ctx, add.targetID, add.normalized, "", "", boolToInt(add.inScope))
			if err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
			id, err := res.LastInsertId()
			if err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
			if existing := existingMap[add.key]; existing != nil {
				existing.NormID = id
				existing.Norm = add.normalized
				existing.NormInScope = add.inScope
			}
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	if len(detUpdates) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `UPDATE targets_expanded SET target_normalized = ?, in_scope = ?, last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, upd := range detUpdates {
			if _, err := stmt.ExecContext(ctx, upd.normalized, boolToInt(upd.inScope), upd.id); err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
			if existing := existingMap[upd.key]; existing != nil {
				existing.Norm = upd.normalized
				existing.NormInScope = upd.inScope
			}
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	if len(variantAdds) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO targets_expanded(target_id, target_normalized, target_ai_normalized, variant_raw, in_scope, first_seen_at, last_seen_at) VALUES(?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, add := range variantAdds {
			res, err := stmt.ExecContext(ctx, add.targetID, add.entry.TargetNormalized, add.variant.AINormalized, add.variant.Raw, boolToInt(add.variant.InScope))
			if err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
			id, err := res.LastInsertId()
			if err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
			if existing := existingMap[add.key]; existing != nil {
				existing.Variants[add.variant.AINormalized] = existingVariant{
					ID:      id,
					Raw:     add.variant.Raw,
					Norm:    add.variant.AINormalized,
					InScope: add.variant.InScope,
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
		stmt, err := tx.PrepareContext(ctx, `UPDATE targets_expanded SET target_normalized = ?, variant_raw = ?, in_scope = ?, last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, upd := range variantUpdates {
			if _, err := stmt.ExecContext(ctx, upd.entry.TargetNormalized, upd.variant.Raw, boolToInt(upd.variant.InScope), upd.id); err != nil {
				stmt.Close()
				tx.Rollback()
				return nil, err
			}
			if existing := existingMap[upd.key]; existing != nil {
				existing.Variants[upd.variant.AINormalized] = existingVariant{
					ID:      upd.id,
					Raw:     upd.variant.Raw,
					Norm:    upd.variant.AINormalized,
					InScope: upd.variant.InScope,
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
		stmt, err := tx.PrepareContext(ctx, `DELETE FROM targets_expanded WHERE id = ?`)
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

	// Batch Deletes and log changes
	if len(toRemove) > 0 {
		tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		delStmt, err := tx.PrepareContext(ctx, `DELETE FROM targets_raw WHERE id = ?`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}

		for _, ex := range toRemove {
			if _, err := delStmt.ExecContext(ctx, ex.ID); err != nil {
				delStmt.Close()
				tx.Rollback()
				return nil, err
			}
		}
		delStmt.Close()
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	return changes, nil
}

// SetProgramIgnoredStatus sets the is_ignored flag for a program.
func (d *DB) SetProgramIgnoredStatus(ctx context.Context, programURL string, ignored bool) error {
	res, err := d.sql.ExecContext(ctx, "UPDATE programs SET is_ignored = ? WHERE url LIKE ?", boolToInt(ignored), fmt.Sprintf("%%%s%%", programURL))
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
	rows, err := d.sql.QueryContext(ctx, "SELECT url FROM programs WHERE platform = ? AND is_ignored = 1", platform)
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
	err := d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM programs WHERE platform = ? AND disabled = 0 AND is_ignored = 0", platform).Scan(&count)
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
		WHERE p.platform = ? AND p.disabled = 0 AND p.is_ignored = 0
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
		if _, err := tx.ExecContext(ctx, `UPDATE programs SET disabled = 1 WHERE id = ?`, p.ID); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("disabling program %d: %w", p.ID, err)
		}

		// Delete associated targets
		if _, err := tx.ExecContext(ctx, `DELETE FROM targets_raw WHERE program_id = ?`, p.ID); err != nil {
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

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO scope_changes(occurred_at, program_url, platform, handle, target_normalized, target_raw, target_ai_normalized, variant_raw, category, in_scope, is_bbp, change_type) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range changes {
		_, err := stmt.ExecContext(ctx, c.OccurredAt, c.ProgramURL, c.Platform, c.Handle, c.TargetNormalized, c.TargetRaw, c.TargetAINormalized, c.VariantRaw, c.Category, boolToInt(c.InScope), boolToInt(c.IsBBP), c.ChangeType)
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
				if variant.HasInScope {
					inScope = variant.InScope
				}
				entry.Variants = append(entry.Variants, EntryVariant{
					Raw:          rawValue,
					AINormalized: variantNorm,
					InScope:      inScope,
				})
			}
		}

		out = append(out, entry)
	}
	return out, nil
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

	if opts.Platform != "" && opts.Platform != "all" {
		where += " AND p.platform = ?"
		args = append(args, opts.Platform)
	}
	if opts.ProgramFilter != "" {
		filter := fmt.Sprintf("%%%s%%", opts.ProgramFilter)
		where += " AND p.url LIKE ?"
		args = append(args, filter)
	}
	if !opts.IncludeOOS {
		where += " AND v.in_scope = 1"
	}
	if !opts.IncludeIgnored {
		where += " AND p.is_ignored = 0"
	}
	if !opts.Since.IsZero() {
		where += " AND v.last_seen_at >= ?"
		args = append(args, opts.Since.UTC())
	}

	query := fmt.Sprintf(`
		SELECT 
			p.url,
			p.platform,
			p.handle,
			CASE 
				WHEN v.target_ai_normalized <> '' THEN v.target_ai_normalized
				ELSE v.target_normalized
			END AS target_normalized,
			CASE
				WHEN v.target_ai_normalized <> '' AND v.variant_raw <> '' THEN v.variant_raw
				WHEN v.target_ai_normalized = '' THEN t.target
				ELSE t.target
			END AS target_raw,
			t.target AS base_target_raw,
			v.target_normalized AS base_target_normalized,
			t.category,
			t.description,
			v.in_scope,
			t.is_bbp,
			0 AS is_historical,
			CASE WHEN v.target_ai_normalized <> '' THEN 'variant' ELSE 'raw' END AS source
		FROM targets_expanded v
		JOIN targets_raw t ON v.target_id = t.id
		JOIN programs p ON t.program_id = p.id
		%s
		ORDER BY 1, 4
	`, where)

	rows, err := d.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var (
			e               Entry
			inScopeInt      int
			isBBPInt        int
			isHistoricalInt int
			baseRawNS       sql.NullString
			descNS          sql.NullString
			source          string
			baseNormNS      sql.NullString
			targetNorm      string
			tgtRaw          string
		)
		if err := rows.Scan(
			&e.ProgramURL,
			&e.Platform,
			&e.Handle,
			&targetNorm,
			&tgtRaw,
			&baseRawNS,
			&baseNormNS,
			&e.Category,
			&descNS,
			&inScopeInt,
			&isBBPInt,
			&isHistoricalInt,
			&source,
		); err != nil {
			return nil, err
		}
		e.TargetNormalized = targetNorm
		e.TargetRaw = tgtRaw
		e.BaseTargetRaw = baseRawNS.String
		e.BaseTargetNormalized = baseNormNS.String
		e.Description = descNS.String
		e.InScope = inScopeInt == 1
		e.IsBBP = isBBPInt == 1
		e.IsHistorical = isHistoricalInt == 1
		e.Source = source
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddCustomTarget adds a single target for a custom program.
func (d *DB) AddCustomTarget(ctx context.Context, target, category string) error {
	platform := "custom"
	// Sanitize target to avoid weird characters in URL
	safeTarget := url.PathEscape(target)
	programURL := "custom://" + safeTarget
	handle := target

	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var programID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM programs WHERE url = ?", programURL).Scan(&programID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.ExecContext(ctx, "INSERT INTO programs(platform, handle, url, first_seen_at, last_seen_at) VALUES(?,?,?,CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", platform, handle, programURL)
		if err != nil {
			return fmt.Errorf("inserting custom program: %w", err)
		}
		programID, err = res.LastInsertId()
		if err != nil {
			return err
		}
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE programs SET last_seen_at = CURRENT_TIMESTAMP, disabled = 0 WHERE id = ?", programID)
		if err != nil {
			return err
		}
	}

	var targetID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM targets_raw WHERE program_id = ? AND target = ?", programID, target).Scan(&targetID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO targets_raw(program_id, target, category, in_scope, is_bbp, first_seen_at, last_seen_at)
			VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			programID, target, category, boolToInt(true), boolToInt(false))
		if err != nil {
			return fmt.Errorf("inserting custom target: %w", err)
		}
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE targets_raw SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?", targetID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListRecentChanges returns the most recent N changes across all programs.
func (d *DB) ListRecentChanges(ctx context.Context, limit int) ([]Change, error) {
	if limit <= 0 {
		limit = 50
	}
	q := "SELECT occurred_at, program_url, platform, handle, target_normalized, target_raw, target_ai_normalized, variant_raw, category, in_scope, is_bbp, change_type FROM scope_changes ORDER BY occurred_at DESC LIMIT ?"
	rows, err := d.sql.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	changes := []Change{}
	for rows.Next() {
		var c Change
		var occurredAtStr string
		var inScopeInt, isBBPInt int
		if err := rows.Scan(&occurredAtStr, &c.ProgramURL, &c.Platform, &c.Handle, &c.TargetNormalized, &c.TargetRaw, &c.TargetAINormalized, &c.VariantRaw, &c.Category, &inScopeInt, &isBBPInt, &c.ChangeType); err != nil {
			return nil, err
		}
		if t, perr := time.Parse("2006-01-02 15:04:05", occurredAtStr); perr == nil {
			c.OccurredAt = t
		} else if t2, perr2 := time.Parse(time.RFC3339, occurredAtStr); perr2 == nil {
			c.OccurredAt = t2
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
			SELECT t.program_id, v.in_scope
			FROM targets_expanded v
			JOIN targets_raw t ON v.target_id = t.id
			WHERE v.target_ai_normalized = ''
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
			CASE 
				WHEN v.target_ai_normalized <> '' THEN v.target_ai_normalized
				ELSE v.target_normalized
			END AS target_normalized,
			CASE
				WHEN v.target_ai_normalized <> '' AND v.variant_raw <> '' THEN v.variant_raw
				ELSE t.target
			END AS target_raw,
			t.target AS base_target_raw,
			v.target_normalized AS base_target_normalized,
			t.category,
			t.description,
			v.in_scope,
			t.is_bbp,
			0 as is_historical,
			CASE WHEN v.target_ai_normalized <> '' THEN 'variant' ELSE 'raw' END AS source
		FROM targets_expanded v
		JOIN targets_raw t ON v.target_id = t.id
		JOIN programs p ON t.program_id = p.id
		WHERE p.is_ignored = 0 AND (
			v.target_normalized LIKE ? OR
			v.target_ai_normalized LIKE ? OR
			t.description LIKE ? OR
			p.url LIKE ?
		)

		UNION

		SELECT 
			c.program_url,
			c.platform,
			c.handle,
			CASE WHEN c.target_ai_normalized <> '' THEN c.target_ai_normalized ELSE c.target_normalized END AS target_normalized,
			CASE WHEN c.variant_raw <> '' THEN c.variant_raw ELSE c.target_raw END AS target_raw,
			c.target_raw AS base_target_raw,
			c.target_normalized AS base_target_normalized,
			c.category,
			NULL as description,
			c.in_scope,
			c.is_bbp,
			1 as is_historical,
			'historical' as source
		FROM scope_changes c
		WHERE c.target_normalized LIKE ? OR c.target_ai_normalized LIKE ? OR c.program_url LIKE ?;
	`

	rows, err := d.sql.QueryContext(ctx, query,
		likeQuery, likeQuery, likeQuery, likeQuery,
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
			e               Entry
			inScopeInt      int
			isHistoricalInt int
			isBBPInt        int
			rawNS           sql.NullString
			baseRawNS       sql.NullString
			baseNormNS      sql.NullString
			descNS          sql.NullString
			source          string
		)
		if err := rows.Scan(
			&e.ProgramURL,
			&e.Platform,
			&e.Handle,
			&e.TargetNormalized,
			&rawNS,
			&baseRawNS,
			&baseNormNS,
			&e.Category,
			&descNS,
			&inScopeInt,
			&isBBPInt,
			&isHistoricalInt,
			&source,
		); err != nil {
			return nil, err
		}
		e.TargetRaw = rawNS.String
		e.BaseTargetRaw = baseRawNS.String
		e.BaseTargetNormalized = baseNormNS.String
		e.Description = descNS.String
		e.InScope = inScopeInt == 1
		e.IsBBP = isBBPInt == 1
		e.IsHistorical = isHistoricalInt == 1
		e.Source = source

		// The UNION can return duplicates, so we'll filter them here.
		key := fmt.Sprintf("%s|%s|%s|%s", e.ProgramURL, e.TargetNormalized, e.BaseTargetNormalized, e.Category)
		if idx, ok := seen[key]; ok {
			// Prefer current entries over historical ones if both exist
			if out[idx].IsHistorical && !e.IsHistorical {
				out[idx] = e
			}
		} else {
			out = append(out, e)
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
