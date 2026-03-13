package core

import (
	"context"
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
	"github.com/sw33tLie/bbscope/v2/pkg/polling"
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

// stdLogger adapts stdlib log.Printf to the polling.Logger interface.
type stdLogger struct{}

func (stdLogger) Infof(format string, args ...interface{})  { log.Printf("[INFO] "+format, args...) }
func (stdLogger) Warnf(format string, args ...interface{})  { log.Printf("[WARN] "+format, args...) }
func (stdLogger) Errorf(format string, args ...interface{}) { log.Printf("[ERROR] "+format, args...) }
func (stdLogger) Debugf(format string, args ...interface{}) { log.Printf("[DEBUG] "+format, args...) }

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
	bcPublicOnly := os.Getenv("BC_PUBLIC_ONLY")
	if bcPublicOnly != "" {
		pollers = append(pollers, bcplatform.NewPollerPublicOnly())
		log.Println("Poller: Bugcrowd public-only mode (no auth)")
	} else if bcEmail != "" && bcPass != "" && bcOTP != "" {
		bcPoller := &bcplatform.Poller{}
		authCfg := platforms.AuthConfig{Email: bcEmail, Password: bcPass, OtpSecret: bcOTP}
		if err := bcPoller.Authenticate(ctx, authCfg); err != nil {
			log.Printf("Poller: Bugcrowd auth failed: %v", err)
			setPollerStatus(&PollerStatus{Platform: "bc", StartedAt: time.Now(), Success: false})
		} else {
			pollers = append(pollers, bcPoller)
		}
	} else {
		log.Println("Poller: Skipping Bugcrowd (BC_EMAIL/BC_PASSWORD/BC_OTP not set, BC_PUBLIC_ONLY not set)")
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

	var wg sync.WaitGroup
	for _, p := range pollers {
		wg.Add(1)
		go func(p platforms.PlatformPoller) {
			defer wg.Done()
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
		}(p)
	}
	wg.Wait()

	invalidateProgramsCache()
	log.Printf("Poll cycle completed in %s", time.Since(start).Round(time.Second))
}

// pollPlatform polls a single platform using the shared polling package.
func pollPlatform(ctx context.Context, p platforms.PlatformPoller, opts platforms.PollOptions, aiNormalizer ai.Normalizer) error {
	log.Printf("Poller: Fetching scope from %s...", p.Name())

	result, err := polling.PollPlatform(ctx, polling.PlatformConfig{
		Poller:      p,
		Options:     opts,
		DB:          db,
		Concurrency: pollConcurrency,
		Normalizer:  aiNormalizer,
		Log:         stdLogger{},
	})
	if err != nil {
		return err
	}

	log.Printf("Poller: Finished %s (%d programs processed)", p.Name(), len(result.PolledProgramURLs))
	return nil
}
