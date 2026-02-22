package core

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
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
