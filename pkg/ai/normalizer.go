package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// ProgramInfo carries minimal details that help the LLM reason about scope entries.
type ProgramInfo struct {
	ProgramURL string
	Platform   string
	Handle     string
}

// Config controls how the AI normalizer behaves.
type Config struct {
	Provider       string
	APIKey         string
	Model          string
	Endpoint       string
	MaxBatch       int
	MaxConcurrency int
	HTTPClient     *http.Client
}

// Normalizer defines the behavior required to transform raw scope targets via LLMs.
type Normalizer interface {
	NormalizeTargets(ctx context.Context, info ProgramInfo, items []storage.TargetItem) ([]storage.TargetItem, error)
}

const (
	defaultProvider       = "openai"
	defaultModel          = "gpt-4.1-mini"
	defaultEndpoint       = "https://api.openai.com/v1/chat/completions"
	defaultMaxBatchSize   = 25
	defaultMaxConcurrency = 10
)

// NewNormalizer builds a concrete Normalizer implementation based on the provided config.
func NewNormalizer(cfg Config) (Normalizer, error) {
	cfg.Provider = strings.TrimSpace(strings.ToLower(cfg.Provider))
	if cfg.Provider == "" {
		cfg.Provider = defaultProvider
	}

	switch cfg.Provider {
	case "openai":
		return newOpenAINormalizer(cfg)
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", cfg.Provider)
	}
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type openAINormalizer struct {
	apiKey         string
	model          string
	endpoint       string
	maxBatchSize   int
	maxConcurrency int
	client         httpClient
}

type normalizedResult struct {
	Targets []string
	InScope *bool
}

func newOpenAINormalizer(cfg Config) (*openAINormalizer, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("ai normalization requires an API key (set ai.api_key in config or OPENAI_API_KEY)")
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	maxBatch := cfg.MaxBatch
	if maxBatch <= 0 {
		maxBatch = defaultMaxBatchSize
	}

	maxConcurrency := cfg.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = defaultMaxConcurrency
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 45 * time.Second}
	}

	return &openAINormalizer{
		apiKey:         apiKey,
		model:          model,
		endpoint:       endpoint,
		maxBatchSize:   maxBatch,
		maxConcurrency: maxConcurrency,
		client:         httpClient,
	}, nil
}

// NormalizeTargets applies AI-powered cleanup, expanding or correcting malformed entries
// while guaranteeing that every original item is preserved.
func (n *openAINormalizer) NormalizeTargets(ctx context.Context, info ProgramInfo, items []storage.TargetItem) ([]storage.TargetItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	utils.Log.Debugf("[ai] starting normalization for %s (%s) - %d targets", info.ProgramURL, info.Handle, len(items))

	type chunkWork struct {
		index int
		start int
		end   int
		items []storage.TargetItem
	}

	var chunks []chunkWork
	for start := 0; start < len(items); start += n.maxBatchSize {
		end := start + n.maxBatchSize
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, chunkWork{
			index: len(chunks),
			start: start,
			end:   end,
			items: items[start:end],
		})
	}

	if len(chunks) == 0 {
		return nil, nil
	}

	results := make([][]storage.TargetItem, len(chunks))

	workerLimit := n.maxConcurrency
	if workerLimit <= 0 {
		workerLimit = 1
	}
	sem := make(chan struct{}, workerLimit)

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	for _, chunk := range chunks {
		wg.Add(1)
		go func(c chunkWork) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			utils.Log.Debugf("[ai] normalizing chunk %d-%d (size %d)", c.start, c.end-1, len(c.items))
			chunkResult, err := n.normalizeChunk(ctx, info, c.start, c.items)
			if err != nil {
				utils.Log.Debugf("[ai] chunk %d-%d failed: %v", c.start, c.end-1, err)
				errOnce.Do(func() { firstErr = err })
				return
			}
			utils.Log.Debugf("[ai] chunk %d-%d normalized into %d targets", c.start, c.end-1, len(chunkResult))
			results[c.index] = chunkResult
		}(chunk)
	}

	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	var out []storage.TargetItem
	for _, chunkRes := range results {
		out = append(out, chunkRes...)
	}

	utils.Log.Debugf("[ai] finished normalization for %s (%s) - expanded to %d targets", info.ProgramURL, info.Handle, len(out))
	return out, nil
}

func (n *openAINormalizer) normalizeChunk(ctx context.Context, info ProgramInfo, baseID int, items []storage.TargetItem) ([]storage.TargetItem, error) {
	normalized, err := n.queryLLM(ctx, info, baseID, items)
	if err != nil {
		return nil, err
	}
	return mergeNormalized(items, baseID, normalized), nil
}

func (n *openAINormalizer) queryLLM(ctx context.Context, info ProgramInfo, baseID int, items []storage.TargetItem) (map[int]normalizedResult, error) {
	payload := llmInput{
		ProgramURL: info.ProgramURL,
		Platform:   info.Platform,
		Handle:     info.Handle,
	}

	for idx, item := range items {
		payload.Items = append(payload.Items, llmInputItem{
			ID:          baseID + idx,
			Target:      item.URI,
			Category:    item.Category,
			Description: item.Description,
			InScope:     item.InScope,
		})
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	reqBody := openAIChatRequest{
		Model: n.model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: string(payloadJSON)},
		},
		Temperature:    0.1,
		ResponseFormat: openAIResponseFormat{Type: "json_object"},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+n.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var apiErrResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErrResp)
		if apiErrResp.Error.Message != "" {
			return nil, fmt.Errorf("ai normalization: %s", apiErrResp.Error.Message)
		}
		return nil, fmt.Errorf("ai normalization failed with HTTP %d", resp.StatusCode)
	}

	var apiResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	if len(apiResp.Choices) == 0 || strings.TrimSpace(apiResp.Choices[0].Message.Content) == "" {
		return nil, errors.New("ai normalization returned an empty response")
	}

	content := strings.TrimSpace(apiResp.Choices[0].Message.Content)

	var parsed llmOutput
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("unable to parse AI response: %w", err)
	}

	result := make(map[int]normalizedResult, len(parsed.Items))
	for _, item := range parsed.Items {
		result[item.ID] = normalizedResult{
			Targets: item.Normalized,
			InScope: item.InScope,
		}
	}

	return result, nil
}

const systemPrompt = `You clean up messy bug bounty scope entries.

For every item you receive:
- Convert everything to lowercase.
- Strip whitespace and obvious typos.
- Expand bracket or pipe notations like "example.(it|com)" into each explicit domain.
- When a scope ends with ".*" assume ".com" (example.* -> example.com, *.example.* -> *.example.com).
- Keep URLs/IPs/CIDRs intact but fix malformed hosts (remove regex, trailing dots, or redundant slashes).
- When the text clearly states "out of scope", "test-only", or similar, set "in_scope": false. If it clearly says "in scope", set true. If unclear, omit the field.
- If unsure how to clean a target, fall back to the provided string exactly.
- Also remove paths from wildcard scope targets. For example, "*.example.com/*" should be cleaned to "example.com".

Return ONLY JSON following this schema:
{
  "items": [
    {"id": 0, "normalized": ["string", "string"], "in_scope": true, "notes": "optional clarification"}
  ]
}

Every input id must appear exactly once and must include at least one normalized string.`

type openAIChatRequest struct {
	Model          string               `json:"model"`
	Messages       []openAIMessage      `json:"messages"`
	Temperature    float64              `json:"temperature"`
	ResponseFormat openAIResponseFormat `json:"response_format"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponseFormat struct {
	Type string `json:"type"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type llmInput struct {
	ProgramURL string         `json:"program_url"`
	Platform   string         `json:"platform"`
	Handle     string         `json:"handle"`
	Items      []llmInputItem `json:"items"`
}

type llmInputItem struct {
	ID          int    `json:"id"`
	Target      string `json:"target"`
	Category    string `json:"category"`
	Description string `json:"description,omitempty"`
	InScope     bool   `json:"in_scope"`
}

type llmOutput struct {
	Items []llmOutputItem `json:"items"`
}

type llmOutputItem struct {
	ID         int      `json:"id"`
	Normalized []string `json:"normalized"`
	InScope    *bool    `json:"in_scope,omitempty"`
	Notes      string   `json:"notes"`
}

func mergeNormalized(items []storage.TargetItem, baseID int, normalized map[int]normalizedResult) []storage.TargetItem {
	if len(items) == 0 {
		return nil
	}

	out := make([]storage.TargetItem, 0, len(items))
	for idx, original := range items {
		id := baseID + idx
		result := normalized[id]
		targets := sanitizeTargets(result.Targets)

		cloned := original
		cloned.Variants = nil
		if result.InScope != nil {
			cloned.InScope = *result.InScope
		}

		if len(targets) > 0 {
			cloned.Variants = make([]storage.TargetVariant, 0, len(targets))
			for _, target := range targets {
				if target == "" {
					continue
				}
				var hasInScope bool
				var inScopeVal bool
				if result.InScope != nil {
					hasInScope = true
					inScopeVal = *result.InScope
				}
				cloned.Variants = append(cloned.Variants, storage.TargetVariant{
					Value:      target,
					HasInScope: hasInScope,
					InScope:    inScopeVal,
				})
			}
		}

		out = append(out, cloned)
	}

	return out
}

func sanitizeTargets(targets []string) []string {
	if len(targets) == 0 {
		return nil
	}

	out := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		trimmed := strings.TrimSpace(strings.ToLower(t))
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
