package polling

import (
	"context"
	"errors"
	"sync"

	"github.com/sw33tLie/bbscope/v2/pkg/ai"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// Logger abstracts logging so callers can use logrus, stdlib log, or any
// other logger that satisfies this interface.
type Logger interface {
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Debugf(format string, args ...interface{})
}

// nopLogger silently discards all messages.
type nopLogger struct{}

func (nopLogger) Infof(string, ...interface{})  {}
func (nopLogger) Warnf(string, ...interface{})  {}
func (nopLogger) Errorf(string, ...interface{}) {}
func (nopLogger) Debugf(string, ...interface{}) {}

// PlatformConfig holds everything PollPlatform needs for a single platform.
type PlatformConfig struct {
	Poller      platforms.PlatformPoller
	Options     platforms.PollOptions
	DB          *storage.DB
	Concurrency int            // defaults to 5 if <= 0
	Normalizer  ai.Normalizer  // optional
	Log         Logger         // optional; nil = no logging

	// OnProgramDone is called per-program after upsert+log (from worker goroutines).
	// Enables CLI to stream-print changes as they happen. Nil = no callback.
	OnProgramDone func(programURL string, changes []storage.Change, isFirstRun bool)
}

// PlatformResult holds the outcome of polling a single platform.
type PlatformResult struct {
	PolledProgramURLs     []string
	ProgramChanges        []storage.Change  // all per-program changes accumulated
	RemovedProgramChanges []storage.Change  // from SyncPlatformPrograms
	IsFirstRun            bool
	Errors                []error           // non-fatal errors
}

// PollPlatform polls a single platform: lists handles, fetches scopes
// concurrently, upserts to DB, syncs removed programs. DB is required.
func PollPlatform(ctx context.Context, cfg PlatformConfig) (*PlatformResult, error) {
	log := cfg.Log
	if log == nil {
		log = nopLogger{}
	}
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}
	p := cfg.Poller
	db := cfg.DB

	result := &PlatformResult{}

	// Determine if this is the first run for this platform.
	programCount, err := db.GetActiveProgramCount(ctx, p.Name())
	if err != nil {
		log.Warnf("Could not get program count for %s: %v", p.Name(), err)
	} else {
		result.IsFirstRun = programCount == 0
	}

	// Load ignored programs.
	ignoredPrograms, err := db.GetIgnoredPrograms(ctx, p.Name())
	if err != nil {
		log.Warnf("Could not get ignored programs for %s: %v", p.Name(), err)
		ignoredPrograms = make(map[string]bool)
	}

	// List program handles from the platform.
	handles, err := p.ListProgramHandles(ctx, cfg.Options)
	if err != nil {
		return nil, err
	}

	if result.IsFirstRun && len(handles) > 0 {
		log.Infof("First poll for %s, populating database...", p.Name())
	}

	// Safety check: if poller returns 0 programs but DB has many, abort to
	// prevent accidentally wiping all programs.
	dbProgramCount, err := db.GetActiveProgramCount(ctx, p.Name())
	if err != nil {
		log.Warnf("Could not get program count for %s: %v", p.Name(), err)
	}
	if len(handles) == 0 && dbProgramCount > 10 {
		log.Errorf("Poller for %s returned 0 programs, but database has %d. Aborting sync for this platform to prevent data loss.", p.Name(), dbProgramCount)
		return result, nil
	}

	// Process all programs concurrently.
	polledURLs, changes, errs := processProgramsConcurrently(ctx, p, handles, cfg.Options, db, ignoredPrograms, result.IsFirstRun, concurrency, cfg.Normalizer, log, cfg.OnProgramDone)
	result.PolledProgramURLs = polledURLs
	result.ProgramChanges = changes
	result.Errors = errs

	// Sync platform programs: mark removed ones as disabled.
	removedChanges, err := db.SyncPlatformPrograms(ctx, p.Name(), polledURLs)
	if err != nil {
		log.Warnf("Failed to sync removed programs for platform %s: %v", p.Name(), err)
	}
	result.RemovedProgramChanges = removedChanges

	if !result.IsFirstRun {
		if err := db.LogChanges(ctx, removedChanges); err != nil {
			log.Warnf("Could not log removed program changes for platform %s: %v", p.Name(), err)
		}
	}

	return result, nil
}

// processProgramsConcurrently fetches and processes programs using a worker pool.
func processProgramsConcurrently(
	ctx context.Context,
	p platforms.PlatformPoller,
	handles []string,
	opts platforms.PollOptions,
	db *storage.DB,
	ignoredPrograms map[string]bool,
	isFirstRun bool,
	concurrency int,
	normalizer ai.Normalizer,
	log Logger,
	onDone func(string, []storage.Change, bool),
) ([]string, []storage.Change, []error) {
	if len(handles) == 0 {
		return []string{}, nil, nil
	}

	handleChan := make(chan string, len(handles))

	var mu sync.Mutex
	polledProgramURLs := make([]string, 0, len(handles))
	var allChanges []storage.Change
	var allErrors []error

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for h := range handleChan {
				changes, err := processOneProgram(ctx, p, h, opts, db, ignoredPrograms, isFirstRun, normalizer, log)
				if err != nil {
					mu.Lock()
					allErrors = append(allErrors, err)
					mu.Unlock()
					continue
				}
				if changes == nil {
					// Program was skipped (ignored).
					continue
				}

				mu.Lock()
				polledProgramURLs = append(polledProgramURLs, changes.programURL)
				allChanges = append(allChanges, changes.changes...)
				mu.Unlock()

				if onDone != nil {
					onDone(changes.programURL, changes.changes, isFirstRun)
				}
			}
		}()
	}

	for _, h := range handles {
		handleChan <- h
	}
	close(handleChan)
	wg.Wait()

	return polledProgramURLs, allChanges, allErrors
}

// programResult is an internal type returned by processOneProgram.
type programResult struct {
	programURL string
	changes    []storage.Change
}

// processOneProgram fetches scope for a single program, builds items,
// optionally applies AI normalization, upserts to DB, and logs changes.
// Returns nil result (no error) when a program is skipped (e.g. ignored).
func processOneProgram(
	ctx context.Context,
	p platforms.PlatformPoller,
	handle string,
	opts platforms.PollOptions,
	db *storage.DB,
	ignoredPrograms map[string]bool,
	isFirstRun bool,
	normalizer ai.Normalizer,
	log Logger,
) (*programResult, error) {
	pd, err := p.FetchProgramScope(ctx, handle, opts)
	if err != nil {
		log.Warnf("Failed to fetch scope for %s: %v", handle, err)
		return nil, err
	}

	if ignoredPrograms[pd.Url] {
		log.Debugf("Skipping ignored program: %s", pd.Url)
		return nil, nil
	}

	allItems := buildTargetItems(pd)

	// Apply AI normalization if available.
	processedItems := allItems
	if normalizer != nil && len(allItems) > 0 {
		processedItems = applyAINormalization(ctx, normalizer, db, pd.Url, p.Name(), handle, allItems, log)
	}

	entries, err := storage.BuildEntries(pd.Url, p.Name(), handle, processedItems)
	if err != nil {
		return nil, err
	}

	changes, err := db.UpsertProgramEntries(ctx, pd.Url, p.Name(), handle, entries)
	if err != nil {
		if errors.Is(err, storage.ErrAbortingScopeWipe) {
			log.Warnf("Potential scope wipe detected for program %s. Skipping update.", pd.Url)
			// Return the URL so it's still counted as polled, but no changes.
			return &programResult{programURL: pd.Url}, nil
		}
		log.Warnf("Database error for program %s: %v", pd.Url, err)
		return nil, err
	}

	if !isFirstRun {
		if err := db.LogChanges(ctx, changes); err != nil {
			log.Warnf("Could not log changes for program %s: %v", pd.Url, err)
		}
	}

	return &programResult{programURL: pd.Url, changes: changes}, nil
}

// buildTargetItems converts ProgramData scope elements into TargetItems.
func buildTargetItems(pd scope.ProgramData) []storage.TargetItem {
	items := make([]storage.TargetItem, 0, len(pd.InScope)+len(pd.OutOfScope))
	for _, s := range pd.InScope {
		items = append(items, storage.TargetItem{URI: s.Target, Category: s.Category, Description: s.Description, InScope: true, IsBBP: s.IsBBP})
	}
	for _, s := range pd.OutOfScope {
		items = append(items, storage.TargetItem{URI: s.Target, Category: s.Category, Description: s.Description, InScope: false, IsBBP: s.IsBBP})
	}
	return items
}

// applyAINormalization runs AI normalization on items, reusing existing
// AI enhancements from the database to avoid redundant API calls.
func applyAINormalization(
	ctx context.Context,
	normalizer ai.Normalizer,
	db *storage.DB,
	programURL, platform, handle string,
	allItems []storage.TargetItem,
	log Logger,
) []storage.TargetItem {
	aiEnhancements, err := db.ListAIEnhancements(ctx, programURL)
	if err != nil {
		log.Warnf("Failed to load AI enhancements for %s: %v", programURL, err)
		aiEnhancements = nil
	}

	var processedItems []storage.TargetItem
	var aiCandidates []storage.TargetItem

	for _, item := range allItems {
		key := storage.BuildTargetCategoryKey(item.URI, item.Category)
		if variants, ok := aiEnhancements[key]; ok && len(variants) > 0 {
			clone := item
			clone.Variants = append([]storage.TargetVariant(nil), variants...)
			processedItems = append(processedItems, clone)
			continue
		}
		aiCandidates = append(aiCandidates, item)
	}

	if len(aiCandidates) > 0 {
		normalized, err := normalizer.NormalizeTargets(ctx, ai.ProgramInfo{
			ProgramURL: programURL,
			Platform:   platform,
			Handle:     handle,
		}, aiCandidates)
		if err != nil {
			log.Warnf("AI normalization failed for %s: %v", programURL, err)
			processedItems = append(processedItems, aiCandidates...)
		} else if len(normalized) > 0 {
			processedItems = append(processedItems, normalized...)
		} else {
			processedItems = append(processedItems, aiCandidates...)
		}
	}

	if len(processedItems) == 0 {
		return allItems
	}

	return processedItems
}
