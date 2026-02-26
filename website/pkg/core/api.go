package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
	"github.com/sw33tLie/bbscope/v2/pkg/targets"
)

// API cache for program list — separate slots for AI-enhanced and raw modes.
var (
	programsCacheMu      sync.RWMutex
	programsCacheAI      []byte    // AI-enhanced (default)
	programsCacheAITime  time.Time
	programsCacheRaw     []byte    // raw mode
	programsCacheRawTime time.Time
	programsCacheTTL     = 5 * time.Minute
)

// invalidateProgramsCache clears both cached program lists so the next request rebuilds them.
func invalidateProgramsCache() {
	programsCacheMu.Lock()
	programsCacheAI = nil
	programsCacheAITime = time.Time{}
	programsCacheRaw = nil
	programsCacheRawTime = time.Time{}
	programsCacheMu.Unlock()
}

type programsAPIResponse struct {
	Programs    json.RawMessage `json:"programs"`
	TotalCount  int             `json:"total_count"`
	GeneratedAt string          `json:"generated_at"`
}

// apiProgramsHandler serves GET /api/v1/programs — the full program list as JSON.
// Pass ?raw=true to get raw target data without AI enhancements.
func apiProgramsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		setCORSHeaders(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawMode := r.URL.Query().Get("raw") == "true"

	// Check cache
	programsCacheMu.RLock()
	var cached []byte
	var cacheTime time.Time
	if rawMode {
		cached = programsCacheRaw
		cacheTime = programsCacheRawTime
	} else {
		cached = programsCacheAI
		cacheTime = programsCacheAITime
	}
	programsCacheMu.RUnlock()

	if cached != nil && time.Since(cacheTime) < programsCacheTTL {
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(cached)
		return
	}

	// Cache miss — rebuild
	ctx := context.Background()
	programs, err := db.ListAllProgramsFlat(ctx, rawMode)
	if err != nil {
		log.Printf("API: Error listing programs: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Apply YWH URL rewrite
	for i := range programs {
		programs[i].URL = strings.ReplaceAll(programs[i].URL, "api.yeswehack.com", "yeswehack.com")
	}

	// Marshal programs array
	programsJSON, err := json.Marshal(programs)
	if err != nil {
		log.Printf("API: Error marshaling programs: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Build envelope
	now := time.Now().UTC()
	resp := programsAPIResponse{
		Programs:    programsJSON,
		TotalCount:  len(programs),
		GeneratedAt: now.Format(time.RFC3339),
	}
	respJSON, err := json.Marshal(resp)
	if err != nil {
		log.Printf("API: Error marshaling response: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Store in cache
	programsCacheMu.Lock()
	if rawMode {
		programsCacheRaw = respJSON
		programsCacheRawTime = now
	} else {
		programsCacheAI = respJSON
		programsCacheAITime = now
	}
	programsCacheMu.Unlock()

	setCORSHeaders(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(respJSON)
}

// apiProgramDetailHandler serves GET /api/v1/programs/{platform}/{handle} — single program detail.
// Pass ?raw=true to get raw target data without AI enhancements.
func apiProgramDetailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		setCORSHeaders(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawMode := r.URL.Query().Get("raw") == "true"

	// Parse path: /api/v1/programs/{platform}/{handle}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/programs/")
	path = strings.TrimSuffix(path, "/")

	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
		return
	}

	platform, err := url.PathUnescape(parts[0])
	if err != nil {
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
		return
	}
	handle, err := url.PathUnescape(parts[1])
	if err != nil {
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
		return
	}

	ctx := context.Background()

	program, err := db.GetProgramByPlatformHandle(ctx, platform, handle)
	if err != nil {
		log.Printf("API: Error fetching program %s/%s: %v", platform, handle, err)
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
		return
	}
	if program == nil {
		program, err = db.GetProgramByPlatformHandleAny(ctx, platform, handle)
		if err != nil {
			log.Printf("API: Error fetching program %s/%s: %v", platform, handle, err)
			setCORSHeaders(w)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"internal server error"}`))
			return
		}
	}
	if program == nil {
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
		return
	}

	targets, err := db.ListProgramTargets(ctx, program.ID, rawMode)
	if err != nil {
		log.Printf("API: Error fetching targets for %s/%s: %v", platform, handle, err)
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
		return
	}

	// For removed programs, reconstruct scope from change history
	if len(targets) == 0 && program.Disabled {
		targets, err = db.ListProgramTargetsFromHistory(ctx, program.Platform, program.Handle)
		if err != nil {
			log.Printf("API: Error fetching historical targets for %s/%s: %v", platform, handle, err)
			setCORSHeaders(w)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"internal server error"}`))
			return
		}
	}

	// Derive isBBP and split in/out of scope
	isBBP := false
	var inScope, outOfScope []programDetailTarget
	for _, t := range targets {
		if t.IsBBP {
			isBBP = true
		}
		dt := programDetailTarget{
			Target:      t.TargetDisplay,
			TargetRaw:   t.TargetRaw,
			Category:    t.Category,
			Description: t.Description,
			IsBBP:       t.IsBBP,
		}
		if t.InScope {
			inScope = append(inScope, dt)
		} else {
			outOfScope = append(outOfScope, dt)
		}
	}

	programURL := strings.ReplaceAll(program.URL, "api.yeswehack.com", "yeswehack.com")

	resp := programDetailResponse{
		Platform:   program.Platform,
		Handle:     program.Handle,
		URL:        programURL,
		IsBBP:      isBBP,
		InScope:    inScope,
		OutOfScope: outOfScope,
	}

	respJSON, err := json.Marshal(resp)
	if err != nil {
		log.Printf("API: Error marshaling program detail: %v", err)
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
		return
	}

	setCORSHeaders(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(respJSON)
}

type programDetailTarget struct {
	Target      string `json:"target"`
	TargetRaw   string `json:"target_raw"`
	Category    string `json:"category"`
	Description string `json:"description"`
	IsBBP       bool   `json:"is_bbp"`
}

type programDetailResponse struct {
	Platform   string                `json:"platform"`
	Handle     string                `json:"handle"`
	URL        string                `json:"url"`
	IsBBP      bool                  `json:"is_bbp"`
	InScope    []programDetailTarget `json:"in_scope"`
	OutOfScope []programDetailTarget `json:"out_of_scope"`
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// --- Targets API ---

var (
	targetsCacheMu   sync.RWMutex
	targetsCache     = make(map[string]targetsCacheEntry)
	targetsCacheTTL  = 5 * time.Minute
	targetsFlight    sync.Map // singleflight: cacheKey -> *sync.Once
)

type targetsCacheEntry struct {
	data []byte
	time time.Time
}

// apiTargetsHandler serves GET /api/v1/targets/{wildcards,domains,urls,ips,cidrs}.
// Query params: scope (in|out|all), platform, type (bbp|vdp), format (json).
func apiTargetsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		setCORSHeaders(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse target type from path
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/targets/")
	suffix = strings.TrimSuffix(suffix, "/")
	validTypes := map[string]bool{"wildcards": true, "domains": true, "urls": true, "ips": true, "cidrs": true}
	if !validTypes[suffix] {
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found. valid types: wildcards, domains, urls, ips, cidrs"}`))
		return
	}

	query := r.URL.Query()
	scopeParam := strings.ToLower(query.Get("scope"))
	if scopeParam == "" {
		scopeParam = "in"
	}
	if scopeParam != "in" && scopeParam != "out" && scopeParam != "all" {
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid scope param. use: in, out, all"}`))
		return
	}
	platformParam := strings.ToLower(query.Get("platform"))
	typeParam := strings.ToLower(query.Get("type"))
	formatParam := strings.ToLower(query.Get("format"))

	// Build cache key from full query
	cacheKey := fmt.Sprintf("%s|%s|%s|%s", suffix, scopeParam, platformParam, typeParam)

	// Check cache first
	targetsCacheMu.RLock()
	entry, ok := targetsCache[cacheKey]
	targetsCacheMu.RUnlock()

	if !ok || time.Since(entry.time) >= targetsCacheTTL {
		// Singleflight: only one goroutine populates cache per key
		once, _ := targetsFlight.LoadOrStore(cacheKey, &sync.Once{})
		once.(*sync.Once).Do(func() {
			defer targetsFlight.Delete(cacheKey)
			populateTargetsCache(cacheKey, suffix, scopeParam, platformParam, typeParam)
		})

		// Re-read from cache
		targetsCacheMu.RLock()
		entry = targetsCache[cacheKey]
		targetsCacheMu.RUnlock()
	}

	setCORSHeaders(w)
	w.Header().Set("Cache-Control", "public, max-age=300")

	if formatParam == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		// Parse cached data back to slice for JSON output
		var items []string
		if len(entry.data) > 0 {
			items = strings.Split(string(entry.data), "\n")
		} else {
			items = []string{}
		}
		json.NewEncoder(w).Encode(items)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if len(entry.data) > 0 {
		w.Write(entry.data)
		w.Write([]byte("\n"))
	}
}

// populateTargetsCache runs the heavy DB query and stores results in cache.
func populateTargetsCache(cacheKey, suffix, scopeParam, platformParam, typeParam string) {
	ctx := context.Background()
	opts := storage.ListOptions{
		Platform:    platformParam,
		IncludeOOS:  scopeParam == "out" || scopeParam == "all",
		ProgramType: typeParam,
	}
	entries, err := db.ListEntries(ctx, opts)
	if err != nil {
		log.Printf("API targets: error listing entries: %v", err)
		return
	}

	var results []string
	switch suffix {
	case "wildcards":
		if scopeParam == "out" {
			sorted := targets.CollectOOSWildcardsSorted(entries)
			for _, r := range sorted {
				results = append(results, r.Domain)
			}
		} else {
			sorted := targets.CollectWildcardsSorted(entries, targets.WildcardOptions{})
			for _, r := range sorted {
				results = append(results, r.Domain)
			}
			if scopeParam == "all" {
				oosSorted := targets.CollectOOSWildcardsSorted(entries)
				seen := make(map[string]struct{}, len(results))
				for _, r := range results {
					seen[r] = struct{}{}
				}
				for _, r := range oosSorted {
					if _, exists := seen[r.Domain]; !exists {
						results = append(results, r.Domain)
					}
				}
			}
		}
	case "domains":
		switch scopeParam {
		case "in":
			results = targets.CollectDomains(entries)
		case "out":
			results = targets.CollectOOSDomains(entries)
		case "all":
			results = targets.CollectDomains(entries)
			oos := targets.CollectOOSDomains(entries)
			results = mergeUniqueSorted(results, oos)
		}
	case "urls":
		switch scopeParam {
		case "in":
			results = targets.CollectURLs(entries)
		case "out":
			results = targets.CollectOOSURLs(entries)
		case "all":
			results = targets.CollectURLs(entries)
			oos := targets.CollectOOSURLs(entries)
			results = mergeUniqueSorted(results, oos)
		}
	case "ips":
		switch scopeParam {
		case "in":
			results = targets.CollectIPs(entries)
		case "out":
			results = targets.CollectOOSIPs(entries)
		case "all":
			results = targets.CollectIPs(entries)
			oos := targets.CollectOOSIPs(entries)
			results = mergeUniqueSorted(results, oos)
		}
	case "cidrs":
		switch scopeParam {
		case "in":
			results = targets.CollectCIDRs(entries)
		case "out":
			results = targets.CollectOOSCIDRs(entries)
		case "all":
			results = targets.CollectCIDRs(entries)
			oos := targets.CollectOOSCIDRs(entries)
			results = mergeUniqueSorted(results, oos)
		}
	}

	data := []byte(strings.Join(results, "\n"))
	targetsCacheMu.Lock()
	targetsCache[cacheKey] = targetsCacheEntry{data: data, time: time.Now()}
	targetsCacheMu.Unlock()
}

// mergeUniqueSorted merges two sorted string slices into a single sorted, deduplicated slice.
func mergeUniqueSorted(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for _, s := range a {
		seen[s] = struct{}{}
	}
	for _, s := range b {
		seen[s] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
