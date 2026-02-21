package core

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"
)

func sitemapHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprint(w, `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)

	ctx := context.Background()
	baseURL := fmt.Sprintf("https://%s", serverDomain)

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

	// Individual program pages
	slugs, err := db.ListAllProgramSlugs(ctx)
	if err != nil {
		log.Printf("Sitemap: Error listing program slugs: %v", err)
	} else {
		for _, s := range slugs {
			path := fmt.Sprintf("/program/%s/%s",
				url.PathEscape(strings.ToLower(s.Platform)),
				url.PathEscape(s.Handle),
			)
			addSitemapURLEntry(path, "weekly", 0.7)
		}
	}

	fmt.Fprint(w, `</urlset>`)
}
