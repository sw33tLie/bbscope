package core

import (
	"context"
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"github.com/sw33tLie/bbscope/v2/pkg/ai"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	bcplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/bugcrowd"
	h1platform "github.com/sw33tLie/bbscope/v2/pkg/platforms/hackerone"
	itplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/intigriti"
	ywhplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/yeswehack"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

const pollConcurrency = 5

// PollerStatus holds the result of the last run for a single platform.
type PollerStatus struct {
	Platform  string
	StartedAt time.Time
	Duration  time.Duration
	Success   bool
	Skipped   bool // true if credentials were not configured
}

var (
	pollerStatuses   = make(map[string]*PollerStatus)
	pollerStatusesMu sync.RWMutex
	aiEnabled        bool
)

func setPollerStatus(s *PollerStatus) {
	pollerStatusesMu.Lock()
	pollerStatuses[s.Platform] = s
	pollerStatusesMu.Unlock()
}

// GetPollerStatuses returns a snapshot of all poller statuses.
func GetPollerStatuses() map[string]*PollerStatus {
	pollerStatusesMu.RLock()
	defer pollerStatusesMu.RUnlock()
	out := make(map[string]*PollerStatus, len(pollerStatuses))
	for k, v := range pollerStatuses {
		cp := *v
		out[k] = &cp
	}
	return out
}

// startBackgroundPoller runs periodic poll cycles in the background.
func startBackgroundPoller(cfg ServerConfig) {
	log.Printf("Starting background poller (interval: %d hours)", cfg.PollInterval)

	// Create AI normalizer if API key is configured
	var aiNormalizer ai.Normalizer
	if cfg.OpenAIAPIKey != "" {
		n, err := ai.NewNormalizer(ai.Config{
			APIKey: cfg.OpenAIAPIKey,
			Model:  cfg.OpenAIModel,
		})
		if err != nil {
			log.Printf("Poller: Failed to create AI normalizer: %v (continuing without AI)", err)
		} else {
			aiNormalizer = n
			aiEnabled = true
			log.Println("Poller: AI normalization enabled")
		}
	} else {
		log.Println("Poller: AI normalization disabled (OPENAI_API_KEY not set)")
	}

	// Run immediately on startup
	runPollCycle(aiNormalizer)

	ticker := time.NewTicker(time.Duration(cfg.PollInterval) * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		runPollCycle(aiNormalizer)
	}
}

// buildPollers creates platform pollers from environment variables.
func buildPollers() []platforms.PlatformPoller {
	ctx := context.Background()
	var pollers []platforms.PlatformPoller

	// HackerOne
	h1User := os.Getenv("H1_USERNAME")
	h1Token := os.Getenv("H1_TOKEN")
	if h1User != "" && h1Token != "" {
		pollers = append(pollers, h1platform.NewPoller(h1User, h1Token))
	} else {
		log.Println("Poller: Skipping HackerOne (H1_USERNAME/H1_TOKEN not set)")
		setPollerStatus(&PollerStatus{Platform: "h1", StartedAt: time.Now(), Skipped: true})
	}

	// Bugcrowd
	bcEmail := os.Getenv("BC_EMAIL")
	bcPass := os.Getenv("BC_PASSWORD")
	bcOTP := os.Getenv("BC_OTP")
	if bcEmail != "" && bcPass != "" && bcOTP != "" {
		bcPoller := &bcplatform.Poller{}
		authCfg := platforms.AuthConfig{Email: bcEmail, Password: bcPass, OtpSecret: bcOTP}
		if err := bcPoller.Authenticate(ctx, authCfg); err != nil {
			log.Printf("Poller: Bugcrowd auth failed: %v", err)
			setPollerStatus(&PollerStatus{Platform: "bc", StartedAt: time.Now(), Success: false})
		} else {
			pollers = append(pollers, bcPoller)
		}
	} else {
		log.Println("Poller: Skipping Bugcrowd (BC_EMAIL/BC_PASSWORD/BC_OTP not set)")
		setPollerStatus(&PollerStatus{Platform: "bc", StartedAt: time.Now(), Skipped: true})
	}

	// Intigriti
	itToken := os.Getenv("IT_TOKEN")
	if itToken != "" {
		itPoller := itplatform.NewPoller()
		if err := itPoller.Authenticate(ctx, platforms.AuthConfig{Token: itToken}); err != nil {
			log.Printf("Poller: Intigriti auth failed: %v", err)
			setPollerStatus(&PollerStatus{Platform: "it", StartedAt: time.Now(), Success: false})
		} else {
			pollers = append(pollers, itPoller)
		}
	} else {
		log.Println("Poller: Skipping Intigriti (IT_TOKEN not set)")
		setPollerStatus(&PollerStatus{Platform: "it", StartedAt: time.Now(), Skipped: true})
	}

	// YesWeHack
	ywhEmail := os.Getenv("YWH_EMAIL")
	ywhPass := os.Getenv("YWH_PASSWORD")
	ywhOTP := os.Getenv("YWH_OTP")
	if ywhEmail != "" && ywhPass != "" && ywhOTP != "" {
		ywhPoller := &ywhplatform.Poller{}
		authCfg := platforms.AuthConfig{Email: ywhEmail, Password: ywhPass, OtpSecret: ywhOTP}
		if err := ywhPoller.Authenticate(ctx, authCfg); err != nil {
			log.Printf("Poller: YesWeHack auth failed: %v", err)
			setPollerStatus(&PollerStatus{Platform: "ywh", StartedAt: time.Now(), Success: false})
		} else {
			pollers = append(pollers, ywhPoller)
		}
	} else {
		log.Println("Poller: Skipping YesWeHack (YWH_EMAIL/YWH_PASSWORD/YWH_OTP not set)")
		setPollerStatus(&PollerStatus{Platform: "ywh", StartedAt: time.Now(), Skipped: true})
	}

	return pollers
}

// runPollCycle runs one complete poll cycle across all configured platforms.
func runPollCycle(aiNormalizer ai.Normalizer) {
	log.Println("Starting poll cycle...")
	start := time.Now()

	pollers := buildPollers()
	if len(pollers) == 0 {
		log.Println("No platform credentials configured. Skipping poll cycle.")
		return
	}

	ctx := context.Background()
	opts := platforms.PollOptions{Categories: "all"}

	for _, p := range pollers {
		pStart := time.Now()
		err := pollPlatform(ctx, p, opts, aiNormalizer)
		setPollerStatus(&PollerStatus{
			Platform:  p.Name(),
			StartedAt: pStart,
			Duration:  time.Since(pStart),
			Success:   err == nil,
		})
		if err != nil {
			log.Printf("Poller: Error polling %s: %v", p.Name(), err)
		}
	}

	invalidateProgramsCache()
	log.Printf("Poll cycle completed in %s", time.Since(start).Round(time.Second))
}

// pollPlatform polls a single platform: lists handles, fetches scopes, upserts to DB.
func pollPlatform(ctx context.Context, p platforms.PlatformPoller, opts platforms.PollOptions, aiNormalizer ai.Normalizer) error {
	log.Printf("Poller: Fetching scope from %s...", p.Name())

	isFirstRun := false
	programCount, err := db.GetActiveProgramCount(ctx, p.Name())
	if err != nil {
		log.Printf("Poller: Could not get program count for %s: %v", p.Name(), err)
	} else {
		isFirstRun = programCount == 0
	}

	ignoredPrograms, err := db.GetIgnoredPrograms(ctx, p.Name())
	if err != nil {
		log.Printf("Poller: Could not get ignored programs for %s: %v", p.Name(), err)
		ignoredPrograms = make(map[string]bool)
	}

	handles, err := p.ListProgramHandles(ctx, opts)
	if err != nil {
		return err
	}

	log.Printf("Poller: %s returned %d program handles", p.Name(), len(handles))

	if isFirstRun && len(handles) > 0 {
		log.Printf("Poller: First poll for %s, populating database...", p.Name())
	}

	// Safety check: if poller returns 0 programs but DB has many, skip to prevent wipe
	if len(handles) == 0 && programCount > 10 {
		log.Printf("Poller: %s returned 0 programs but database has %d. Aborting sync to prevent data loss.", p.Name(), programCount)
		return nil
	}

	// Process programs concurrently
	polledProgramURLs := processProgramsConcurrently(ctx, p, handles, opts, ignoredPrograms, isFirstRun, aiNormalizer)

	// Sync platform programs (mark missing programs as disabled)
	removedProgramChanges, err := db.SyncPlatformPrograms(ctx, p.Name(), polledProgramURLs)
	if err != nil {
		log.Printf("Poller: Failed to sync removed programs for %s: %v", p.Name(), err)
	}
	if !isFirstRun {
		if err := db.LogChanges(ctx, removedProgramChanges); err != nil {
			log.Printf("Poller: Could not log removed program changes for %s: %v", p.Name(), err)
		}
	}

	log.Printf("Poller: Finished %s (%d programs processed)", p.Name(), len(polledProgramURLs))
	return nil
}

// processProgramsConcurrently fetches and processes programs using a worker pool.
func processProgramsConcurrently(ctx context.Context, p platforms.PlatformPoller, handles []string, opts platforms.PollOptions, ignoredPrograms map[string]bool, isFirstRun bool, aiNormalizer ai.Normalizer) []string {
	if len(handles) == 0 {
		return []string{}
	}

	handleChan := make(chan string, len(handles))
	var mu sync.Mutex
	polledProgramURLs := make([]string, 0, len(handles))

	var wg sync.WaitGroup
	for i := 0; i < pollConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for h := range handleChan {
				pd, err := p.FetchProgramScope(ctx, h, opts)
				if err != nil {
					log.Printf("Poller: Failed to fetch scope for %s/%s: %v", p.Name(), h, err)
					continue
				}

				if ignoredPrograms[pd.Url] {
					continue
				}

				mu.Lock()
				polledProgramURLs = append(polledProgramURLs, pd.Url)
				mu.Unlock()

				// Build storage items
				var allItems []storage.TargetItem
				for _, s := range pd.InScope {
					allItems = append(allItems, storage.TargetItem{URI: s.Target, Category: s.Category, Description: s.Description, InScope: true, IsBBP: s.IsBBP})
				}
				for _, s := range pd.OutOfScope {
					allItems = append(allItems, storage.TargetItem{URI: s.Target, Category: s.Category, Description: s.Description, InScope: false, IsBBP: s.IsBBP})
				}

				// Apply AI normalization if available
				processedItems := allItems
				if aiNormalizer != nil && len(allItems) > 0 {
					processedItems = applyAINormalization(ctx, aiNormalizer, pd.Url, p.Name(), h, allItems)
				}

				entries, err := storage.BuildEntries(pd.Url, p.Name(), h, processedItems)
				if err != nil {
					log.Printf("Poller: Failed to build entries for %s: %v", pd.Url, err)
					continue
				}

				changes, err := db.UpsertProgramEntries(ctx, pd.Url, p.Name(), h, entries)
				if err != nil {
					if errors.Is(err, storage.ErrAbortingScopeWipe) {
						log.Printf("Poller: Potential scope wipe detected for %s. Skipping update.", pd.Url)
						continue
					}
					log.Printf("Poller: Database error for %s: %v", pd.Url, err)
					continue
				}

				if !isFirstRun {
					if err := db.LogChanges(ctx, changes); err != nil {
						log.Printf("Poller: Could not log changes for %s: %v", pd.Url, err)
					}
				}
			}
		}()
	}

	for _, h := range handles {
		handleChan <- h
	}
	close(handleChan)
	wg.Wait()

	return polledProgramURLs
}

// applyAINormalization runs AI normalization on items, reusing existing AI enhancements
// from the database to avoid redundant API calls.
func applyAINormalization(ctx context.Context, normalizer ai.Normalizer, programURL, platform, handle string, allItems []storage.TargetItem) []storage.TargetItem {
	// Load existing AI enhancements to avoid re-processing unchanged targets
	aiEnhancements, err := db.ListAIEnhancements(ctx, programURL)
	if err != nil {
		log.Printf("Poller: Failed to load AI enhancements for %s: %v", programURL, err)
		aiEnhancements = nil
	}

	var processedItems []storage.TargetItem
	var aiCandidates []storage.TargetItem

	// Separate items with existing AI data from those needing normalization
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

	// Normalize only new/changed targets
	if len(aiCandidates) > 0 {
		normalized, err := normalizer.NormalizeTargets(ctx, ai.ProgramInfo{
			ProgramURL: programURL,
			Platform:   platform,
			Handle:     handle,
		}, aiCandidates)
		if err != nil {
			log.Printf("Poller: AI normalization failed for %s: %v", programURL, err)
			processedItems = append(processedItems, aiCandidates...)
		} else if len(normalized) > 0 {
			processedItems = append(processedItems, normalized...)
		} else {
			processedItems = append(processedItems, aiCandidates...)
		}
	}

	// Fallback: if nothing was processed, return raw items
	if len(processedItems) == 0 {
		return allItems
	}

	return processedItems
}
