package core

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"sort"
)

// generatePageURL constructs a URL with a page parameter, omitting it if page is 1.
func generatePageURL(basePath string, page int) string {
	if page <= 1 {
		return basePath
	}
	return fmt.Sprintf("%s?page=%d", basePath, page)
}

func sitemapHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprint(w, `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)

	baseURL := fmt.Sprintf("https://%s", serverDomain)
	sitemapPerPage := 50

	addSitemapURLEntry := func(path string, changefreq string, priority float64) {
		escapedPath := html.EscapeString(path)
		fmt.Fprintf(w, "<url><loc>%s%s</loc><changefreq>%s</changefreq><priority>%.1f</priority></url>\n", baseURL, escapedPath, changefreq, priority)
	}

	// Static pages
	addSitemapURLEntry("/", "weekly", 1.0)
	addSitemapURLEntry("/scope", "daily", 0.9)
	addSitemapURLEntry("/updates", "daily", 0.9)
	addSitemapURLEntry("/stats", "weekly", 0.7)
	addSitemapURLEntry("/docs", "weekly", 0.7)
	addSitemapURLEntry("/contact", "monthly", 0.6)

	// Dynamic /scope pages (Compact View Only)
	allEntries, err := loadScopeFromDB()
	if err != nil {
		log.Printf("Sitemap: Error loading scope data: %v. Scope URLs will be incomplete.", err)
	} else {
		programSummaries := aggregateAndFilterScopeData(allEntries, "")
		totalResultsCompact := len(programSummaries)
		if totalResultsCompact > 0 {
			totalPagesCompact := (totalResultsCompact + sitemapPerPage - 1) / sitemapPerPage
			for page := 1; page <= totalPagesCompact; page++ {
				addSitemapURLEntry(generatePageURL("/scope", page), "daily", 0.8)
			}
		}
	}

	// Dynamic /updates pages
	allUpdates, errUpdates := loadUpdatesFromDB()
	if errUpdates != nil {
		log.Printf("Sitemap: Error loading updates data: %v. Update URLs will be incomplete.", errUpdates)
	} else {
		sort.SliceStable(allUpdates, func(i, j int) bool {
			return allUpdates[i].Timestamp.After(allUpdates[j].Timestamp)
		})
		totalUpdatesResults := len(allUpdates)
		if totalUpdatesResults > 0 {
			totalPagesUpdates := (totalUpdatesResults + sitemapPerPage - 1) / sitemapPerPage
			for page := 1; page <= totalPagesUpdates; page++ {
				addSitemapURLEntry(generatePageURL("/updates", page), "daily", 0.8)
			}
		}
	}

	fmt.Fprint(w, `</urlset>`)
}
