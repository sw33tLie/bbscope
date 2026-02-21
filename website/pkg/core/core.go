package core

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
	g "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html" // Using . import for convenience with html tags
)

// ServerConfig holds configuration for the web server.
type ServerConfig struct {
	DevMode      bool
	DBUrl        string
	PollInterval int
	ListenAddr   string
	Domain       string
}

var db *storage.DB
var serverDomain string

// UpdateEntryAsset represents an asset within an update event.
type UpdateEntryAsset struct {
	Category string
	Value    string
}

// UpdateEntry represents a single change event.
type UpdateEntry struct {
	Type             string
	ScopeType        string // "In Scope", "Out of Scope", "" for program types
	Asset            UpdateEntryAsset
	ProgramURL       string
	Platform         string
	Timestamp        time.Time
	AssociatedAssets []UpdateEntryAsset // Populated during processing for program_added/removed
}


// loadUpdatesFromDB loads recent changes from the database.
func loadUpdatesFromDB() ([]UpdateEntry, error) {
	ctx := context.Background()
	changes, err := db.ListRecentChanges(ctx, 10000)
	if err != nil {
		return nil, fmt.Errorf("failed to load updates from database: %w", err)
	}

	var updates []UpdateEntry
	for _, c := range changes {
		programURL := strings.ReplaceAll(c.ProgramURL, "api.yeswehack.com", "yeswehack.com")
		category := strings.ToUpper(scope.NormalizeCategory(c.Category))

		var entryType string
		var scopeType string
		if c.Category == "program" {
			if c.ChangeType == "added" {
				entryType = "program_added"
			} else {
				entryType = "program_removed"
			}
		} else {
			if c.ChangeType == "added" {
				entryType = "asset_added"
			} else {
				entryType = "asset_removed"
			}
			if c.InScope {
				scopeType = "In Scope"
			} else {
				scopeType = "Out of Scope"
			}
		}

		target := c.TargetNormalized
		if target == "" {
			target = c.TargetRaw
		}

		updates = append(updates, UpdateEntry{
			Type:       entryType,
			ScopeType:  scopeType,
			Asset:      UpdateEntryAsset{Category: category, Value: target},
			ProgramURL: programURL,
			Platform:   c.Platform,
			Timestamp:  c.OccurredAt,
		})
	}
	return updates, nil
}

// Page layout component
func PageLayout(title, description string, navbar g.Node, content g.Node, footer g.Node, canonicalURL string, shouldNoIndex bool) g.Node {
	return g.Group([]g.Node{
		g.Raw("<!DOCTYPE html>"),
		HTML(
			Head(
				Meta(Charset("UTF-8")),
				Meta(Name("viewport"), Content("width=device-width, initial-scale=1.0")),
				Meta(Name("description"), Content(description)),
				TitleEl(g.Text(title)), // Using TitleEl to avoid conflict
				Script(Src("https://cdn.tailwindcss.com")),
			Script(Src("https://unpkg.com/htmx.org@2.0.4")),
				Script(g.Raw(`tailwind.config={theme:{extend:{colors:{'bb-dark':'#0f172a','bb-blue':'#3b82f6','bb-accent':'#06b6d4','bb-surface':'#1e293b'}}}}`)),

				// Favicon links
				Link(Rel("shortcut icon"), Href("/static/favicon.ico")),
				Link(Rel("icon"), Type("image/png"), g.Attr("sizes", "96x96"), Href("/static/favicon-96x96.png")),
				Link(Rel("icon"), Type("image/x-icon"), Href("/static/favicon.ico")), // Corrected type
				Link(Rel("apple-touch-icon"), g.Attr("sizes", "180x180"), Href("/static/apple-touch-icon.png")),
				Link(Rel("manifest"), Href("/static/site.webmanifest")),
				// Canonical URL
				g.If(canonicalURL != "",
					Link(Rel("canonical"), Href(canonicalURL)),
				),
				// Noindex meta tag for detailed view pages
				g.If(shouldNoIndex,
					Meta(Name("robots"), Content("noindex, follow")),
				),

				// Custom CSS can be added here if needed
				StyleEl(g.Raw(`
					::-webkit-scrollbar { width: 8px; height: 8px; }
					::-webkit-scrollbar-track { background: #1e293b; border-radius: 10px; }
					::-webkit-scrollbar-thumb { background: #475569; border-radius: 10px; }
					::-webkit-scrollbar-thumb:hover { background: #64748b; }
					.table-fixed { table-layout: fixed; }
					.prose { --tw-prose-body: #cbd5e1; --tw-prose-headings: #e2e8f0; --tw-prose-links: #22d3ee; --tw-prose-bold: #e2e8f0; --tw-prose-code: #e2e8f0; --tw-prose-pre-bg: #0f172a; }
				`)),
			),
			Body(Class("bg-slate-950 font-sans leading-normal tracking-normal flex flex-col min-h-screen text-slate-200"),
				navbar,
				Div(Class("flex-grow"), content), // Ensure content pushes footer down
				footer,
				// JavaScript for UI interactions
				Script(g.Raw(`
					// Dropdown menu toggle for download button
					const downloadMenuButton = document.getElementById('download-menu-button');
					const downloadMenu = document.getElementById('download-menu');
					if (downloadMenuButton && downloadMenu) {
						downloadMenuButton.addEventListener('click', (event) => {
							event.stopPropagation(); // Prevent click from bubbling to document listener immediately
							const isExpanded = downloadMenuButton.getAttribute('aria-expanded') === 'true';
							downloadMenuButton.setAttribute('aria-expanded', String(!isExpanded));
							downloadMenu.classList.toggle('hidden');
						});
						// Close dropdown if clicked outside
						document.addEventListener('click', (event) => {
							if (downloadMenu && !downloadMenu.classList.contains('hidden')) {
								if (!downloadMenuButton.contains(event.target) && !downloadMenu.contains(event.target)) {
									downloadMenuButton.setAttribute('aria-expanded', 'false');
									downloadMenu.classList.add('hidden');
								}
							}
						});
					}

					// Mobile menu toggle
					const mobileMenuButton = document.getElementById('mobile-menu-button');
					const mobileMenu = document.getElementById('mobile-menu');
					if (mobileMenuButton && mobileMenu) {
						mobileMenuButton.addEventListener('click', () => {
							const isExpanded = mobileMenuButton.getAttribute('aria-expanded') === 'true';
							mobileMenuButton.setAttribute('aria-expanded', String(!isExpanded));
							mobileMenu.classList.toggle('hidden');
							// TODO: Optionally swap hamburger/close icons here if you add them back
						});
					}

					// Scope details toggle function for compact view
					function toggleScopeDetails(detailsId, iconId) {
						const detailsRow = document.getElementById(detailsId);
						const icon = document.getElementById(iconId);
						if (detailsRow) {
							detailsRow.classList.toggle('hidden');
							if (icon) {
								icon.textContent = detailsRow.classList.contains('hidden') ? '+' : '-';
							}
						}
					}
				`)),
			),
		),
	})
}

// Navbar component
func Navbar() g.Node {
	return Nav(Class("bg-slate-900 text-white p-4 shadow-lg shadow-slate-950/50 sticky top-0 z-50 border-b border-slate-800"),
		Div(Class("container mx-auto flex justify-between items-center"),
			// Logo/Site Name
			A(Href("/"), Class("text-2xl font-bold hover:text-slate-300 transition-colors"), g.Text("bbscope.com")),

			// Mobile Menu Button (Hamburger)
			Div(Class("md:hidden"), // Visible on small screens, hidden on medium and up
				Button(
					ID("mobile-menu-button"), // ID for JavaScript
					Type("button"),
					Class("inline-flex items-center justify-center p-2 rounded-md text-slate-400 hover:text-white hover:bg-slate-700 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-white"),
					g.Attr("aria-controls", "mobile-menu"),
					g.Attr("aria-expanded", "false"), // This would ideally be toggled by JS too
					Span(Class("sr-only"), g.Text("Open main menu")),
					// Icon when menu is closed (Heroicon: menu)
					g.Raw(`<svg class="block h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" /></svg>`),
					// Icon when menu is open (Heroicon: x) -  You'd need JS to swap this
					// RawSVG(`<svg class="hidden h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" /></svg>`),
				),
			),

			// Navigation Links
			// Hidden on small screens, flex on medium and up.
			// Added ID for JS, and classes for mobile dropdown appearance.
			Div(
				ID("mobile-menu"), // ID for JavaScript
				Class("hidden md:flex md:items-center md:space-x-1 w-full md:w-auto absolute md:relative top-16 left-0 md:top-auto md:left-auto bg-slate-900 md:bg-transparent shadow-lg md:shadow-none rounded-b-md md:rounded-none py-2 md:py-0"),
				A(Href("/"), Class("block text-center md:inline-block hover:bg-slate-700 md:hover:bg-transparent hover:text-white md:hover:text-slate-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("Home")),
				A(Href("/scope"), Class("block text-center md:inline-block hover:bg-slate-700 md:hover:bg-transparent hover:text-white md:hover:text-slate-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("Scope")),     // New Scope Link
				A(Href("/updates"), Class("block text-center md:inline-block hover:bg-slate-700 md:hover:bg-transparent hover:text-white md:hover:text-slate-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("Updates")), // New Updates Link
				A(Href("/stats"), Class("block text-center md:inline-block hover:bg-slate-700 md:hover:bg-transparent hover:text-white md:hover:text-slate-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("Stats")),     // New Stats Link
				A(Href("/docs"), Class("block text-center md:inline-block hover:bg-slate-700 md:hover:bg-transparent hover:text-white md:hover:text-slate-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("Docs")),
				A(Href("https://github.com/sw33tLie/bbscope"), Target("_blank"), Rel("noopener noreferrer"), Class("block text-center md:inline-block hover:bg-slate-700 md:hover:bg-transparent hover:text-white md:hover:text-slate-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("GitHub")),
				A(Href("/contact"), Class("block text-center md:inline-block hover:bg-slate-700 md:hover:bg-transparent hover:text-white md:hover:text-slate-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("Contact")),
			),
		),
	)
}

// MainContent component for the landing page
func statCounter(value int, label string) g.Node {
	return Div(Class("text-center"),
		Div(Class("text-3xl md:text-4xl font-bold text-cyan-400 tabular-nums"), g.Text(fmt.Sprintf("%d", value))),
		Div(Class("text-sm text-slate-400 mt-1"), g.Text(label)),
	)
}

func MainContent(totalPrograms, totalAssets, platformCount int) g.Node {
	newHeroSection := Section(
		Div(Class("relative items-center w-full px-5 py-12 mx-auto md:px-12 lg:px-16 max-w-7xl lg:py-24"),
			Div(Class("flex w-full mx-auto text-left"),
				Div(Class("relative inline-flex items-center mx-auto align-middle"),
					Div(Class("text-center"),
						Img( // Added logo here
							Class("block mx-auto mb-6 h-24 w-auto invert"),
							Src("/static/images/bbscope-logo.svg"),
							Alt("bbscope.com logo"),
						),
						H1(Class("max-w-5xl text-2xl font-bold leading-none tracking-tighter text-slate-100 md:text-5xl lg:text-6xl lg:max-w-7xl"),
							g.Text("Bug Bounty Scope Data Aggregator"),
						),
						P(Class("max-w-xl mx-auto mt-8 text-base leading-relaxed text-slate-400"), g.Raw("This website collects public bug bounty targets fetched with <a href='https://github.com/sw33tLie/bbscope' class='text-cyan-400 hover:text-cyan-300 underline'>bbscope cli</a>.<br>We have a few extra tools too!")),
						Div(Class("flex justify-center items-center w-full max-w-2xl gap-2 mx-auto mt-6"), // Added items-center for button alignment
							Div(Class("mt-3 rounded-lg sm:mt-0"),
								A(Href("/scope"), Class("px-5 py-4 text-base font-medium text-center text-white transition duration-500 ease-in-out transform bg-cyan-600 lg:px-10 rounded-xl hover:bg-cyan-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-cyan-500"), g.Text("View scope")),
							),
							Div(Class("mt-3 rounded-lg sm:mt-0 sm:ml-3"),
								A(Href("/updates"), Class("items-center block px-5 lg:px-10 py-3.5 text-base font-medium text-center text-cyan-400 transition duration-500 ease-in-out transform border-2 border-slate-600 shadow-md rounded-xl hover:border-cyan-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-slate-500"), g.Text("Latest changes")),
							),
						),
						Div(Class("flex flex-wrap justify-center gap-8 mt-12"),
							statCounter(totalPrograms, "Programs Tracked"),
							statCounter(totalAssets, "Total Assets"),
							statCounter(platformCount, "Platforms"),
						),
					),
				),
			),
		),
	)

	return Main(Class("container mx-auto mt-8 mb-16 p-4"),
		newHeroSection, // Added the new hero section here

		// Features Section
		Section(Class("py-8 mb-12"),
			H2(Class("text-3xl font-bold text-center text-slate-100 mb-10"), g.Text("Use cases")),
			Div(Class("grid md:grid-cols-3 gap-8"),
				featureCard("Quick Scope", "You can quickly view and download bug bounty scope and use it for your own purposes"),
				featureCard("Track changes", "We track all scope changes. Hunt on fresh targets!"),
				featureCard("Stats", "Get platform insights and statistics for bug bounty programs."),
			),
		),

		// Call to Action Section
		Section(Class("py-12 bg-slate-800/50 border border-slate-700 rounded-lg"),
			Div(Class("container mx-auto text-center px-4"),
				H2(Class("text-3xl font-bold text-slate-100 mb-4"), g.Text("Want to help?")),
				P(Class("text-slate-400 mb-8 max-w-xl mx-auto"), g.Text("This tool is powered by the bbscope CLI tool. Pull requests are welcome!")),
				A(Href("https://github.com/sw33tLie/bbscope"), Target("_blank"), Rel("noopener noreferrer"), Class("bg-emerald-500 hover:bg-emerald-400 text-white font-bold py-3 px-8 rounded-lg text-lg transition duration-300 ease-in-out transform hover:scale-105"),
					g.Text("Go to GitHub"),
				),
			),
		),
	)
}

// platformFilterTabs renders pill-style platform filter tabs.
func platformFilterTabs(basePath, currentPlatform, extraParams string) g.Node {
	platforms := []struct{ Value, Label string }{
		{"", "All"},
		{"hackerone", "HackerOne"},
		{"bugcrowd", "Bugcrowd"},
		{"intigriti", "Intigriti"},
		{"yeswehack", "YesWeHack"},
	}

	tabs := []g.Node{}
	for _, p := range platforms {
		isActive := currentPlatform == p.Value
		href := basePath + "?page=1"
		if p.Value != "" {
			href += "&platform=" + p.Value
		}
		href += extraParams

		classes := "px-4 py-2 text-sm font-medium rounded-lg transition-colors "
		if isActive {
			classes += "bg-cyan-500 text-white shadow-lg shadow-cyan-500/25"
		} else {
			classes += "bg-slate-800 text-slate-400 hover:bg-slate-700 hover:text-slate-200 border border-slate-700"
		}

		tabs = append(tabs, A(Href(href), Class(classes), g.Text(p.Label)))
	}

	return Div(Class("flex flex-wrap gap-2 mb-6"), g.Group(tabs))
}

// Helper function for feature cards
func featureCard(title, description string) g.Node {
	return Div(Class("bg-slate-800/50 border border-slate-700 shadow-lg rounded-lg p-6 hover:border-cyan-500/50 hover:shadow-cyan-500/10 transition-all duration-300"),
		H3(Class("text-xl font-semibold text-slate-200 mb-3"), g.Text(title)),
		P(Class("text-slate-400 text-sm"), g.Text(description)),
	)
}

// FooterEl component (using El suffix to avoid conflict with html.Footer)
func FooterEl() g.Node {
	currentYear := time.Now().Year()
	return Footer(Class("bg-slate-900 text-slate-400 text-center p-6 mt-auto border-t border-slate-800"),
		Div(Class("container mx-auto"),
			P(g.Raw(fmt.Sprintf("© %d bbscope.com. All rights reserved. Made by <a href='https://x.com/sw33tLie' class='hover:text-cyan-300 underline'>sw33tLie</a>", currentYear))),
			Div(Class("mt-2"),
				A(Href("#privacy"), Class("text-slate-500 hover:text-white mx-2 text-sm"), g.Text("Privacy Policy")),
				A(Href("#terms"), Class("text-slate-500 hover:text-white mx-2 text-sm"), g.Text("Terms of Service")),
			),
		),
	)
}

// ScopeContent renders the full /scope page content (used for non-HTMX requests).
func ScopeContent(result *storage.ProgramListResult, loadErr error, search, sortBy, sortOrder string, perPage int, platform string) g.Node {
	pageContent := []g.Node{
		H1(Class("text-3xl md:text-4xl font-bold text-slate-100 mb-6"), g.Text("Scope Data")),
		scopePlatformFilterDropdown(platform, perPage),
		scopeSearchBar(search, sortBy, sortOrder, perPage, platform),
	}

	if loadErr != nil {
		pageContent = append(pageContent,
			Div(Class("bg-red-900/30 border border-red-700 text-red-400 px-4 py-3 rounded relative mb-4"),
				Strong(g.Text("Error: ")),
				g.Text("Could not load scope data. "+loadErr.Error()),
			),
		)
	}

	pageContent = append(pageContent,
		Div(ID("scope-table-container"),
			scopeTableInner(result, loadErr, search, sortBy, sortOrder, perPage, platform),
		),
	)

	return Main(Class("container mx-auto mt-8 mb-16 p-4"),
		Section(Class("bg-slate-900/50 border border-slate-800 rounded-lg shadow-xl p-6 md:p-8 lg:p-12"),
			g.Group(pageContent),
		),
	)
}

// ScopeTableFragment renders just the table container content for HTMX partial responses.
func ScopeTableFragment(result *storage.ProgramListResult, loadErr error, search, sortBy, sortOrder string, perPage int, platform string) g.Node {
	return scopeTableInner(result, loadErr, search, sortBy, sortOrder, perPage, platform)
}

// scopeTableInner renders the controls, table, and pagination for the scope page.
func scopeTableInner(result *storage.ProgramListResult, loadErr error, search, sortBy, sortOrder string, perPage int, platform string) g.Node {
	if loadErr != nil {
		return Div()
	}

	content := []g.Node{}

	// Results count + per-page selector
	var resultsCountText string
	if result.TotalCount > 0 {
		resultsCountText = fmt.Sprintf("Showing %d to %d of %d programs.",
			min((result.Page-1)*result.PerPage+1, result.TotalCount),
			min(result.Page*result.PerPage, result.TotalCount),
			result.TotalCount,
		)
	} else if search != "" {
		resultsCountText = fmt.Sprintf("No programs found for '%s'.", search)
	} else {
		resultsCountText = "No programs to display."
	}

	controlsRow := Div(Class("flex flex-col sm:flex-row justify-between items-center mb-4 gap-4"),
		Div(Class("text-sm text-slate-400"), g.Text(resultsCountText)),
		Div(Class("flex flex-col items-stretch gap-2 sm:flex-row sm:items-center sm:gap-3 w-full sm:w-auto"),
			scopePerPageSelector(search, sortBy, sortOrder, perPage, platform),
		),
	)
	content = append(content, controlsRow)

	// Pagination top
	if result.TotalPages > 1 {
		content = append(content, Div(Class("mb-6 flex justify-center"),
			scopePagination(result.Page, result.TotalPages, search, sortBy, sortOrder, perPage, platform),
		))
	}

	// Build sort URL helper
	buildSortURL := func(targetSortBy string) string {
		order := "asc"
		if sortBy == targetSortBy {
			if sortOrder == "asc" {
				order = "desc"
			} else {
				order = "asc"
			}
		}
		u := fmt.Sprintf("/scope?page=1&sortBy=%s&sortOrder=%s&perPage=%d", targetSortBy, order, perPage)
		if search != "" {
			u += "&search=" + url.QueryEscape(search)
		}
		if platform != "" {
			u += "&platform=" + platform
		}
		return u
	}

	sortIndicator := func(col string) string {
		if sortBy == col {
			if sortOrder == "asc" {
				return " ▲"
			}
			return " ▼"
		}
		return ""
	}

	htmxAttrs := func(href string) []g.Node {
		return []g.Node{
			Href(href),
			g.Attr("hx-get", href),
			g.Attr("hx-target", "#scope-table-container"),
			g.Attr("hx-push-url", "true"),
		}
	}

	// Table headers
	tableHeaders := Tr(
		Th(Class("px-4 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider w-2/5"),
			A(append(htmxAttrs(buildSortURL("handle")), Class("hover:text-slate-200 transition-colors"), g.Text("Program"+sortIndicator("handle")))...),
		),
		Th(Class("px-4 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider w-1/5"),
			A(append(htmxAttrs(buildSortURL("platform")), Class("hover:text-slate-200 transition-colors"), g.Text("Platform"+sortIndicator("platform")))...),
		),
		Th(Class("px-4 py-3 text-center text-xs font-medium text-slate-400 uppercase tracking-wider w-1/5"),
			A(append(htmxAttrs(buildSortURL("in_scope_count")), Class("hover:text-slate-200 transition-colors"), g.Text("In Scope"+sortIndicator("in_scope_count")))...),
		),
		Th(Class("px-4 py-3 text-center text-xs font-medium text-slate-400 uppercase tracking-wider w-1/5"),
			A(append(htmxAttrs(buildSortURL("out_of_scope_count")), Class("hover:text-slate-200 transition-colors"), g.Text("Out of Scope"+sortIndicator("out_of_scope_count")))...),
		),
	)

	// Table rows
	var tableRows []g.Node
	if len(result.Programs) == 0 {
		noResultsMsg := "No programs to display."
		if search != "" {
			noResultsMsg = fmt.Sprintf("No programs found for '%s'.", search)
		}
		tableRows = append(tableRows,
			Tr(Td(ColSpan("4"), Class("text-center py-10 text-slate-400"), g.Text(noResultsMsg))),
		)
	} else {
		for i, p := range result.Programs {
			rowBg := ""
			if i%2 == 1 {
				rowBg = " bg-slate-800/30"
			}

			programURL := fmt.Sprintf("/program/%s/%s",
				url.PathEscape(strings.ToLower(p.Platform)),
				url.PathEscape(p.Handle),
			)
			externalURL := strings.ReplaceAll(p.URL, "api.yeswehack.com", "yeswehack.com")

			tableRows = append(tableRows,
				Tr(
					Class("border-b border-slate-700/50 hover:bg-slate-800/70 transition-colors cursor-pointer"+rowBg),
					g.Attr("onclick", fmt.Sprintf("window.location.href='%s'", programURL)),
					Td(Class("px-4 py-3 text-sm text-slate-200 w-2/5"),
						Div(Class("flex items-center gap-2"),
							A(Href(externalURL), Target("_blank"), Rel("noopener noreferrer"),
								Class("text-slate-500 hover:text-cyan-400 transition-colors flex-shrink-0"),
								g.Attr("onclick", "event.stopPropagation()"),
								g.Attr("title", "Open program page"),
								g.Raw(`<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"/></svg>`),
							),
							Span(Class("font-medium text-slate-100"), g.Text(p.Handle)),
						),
					),
					Td(Class("px-4 py-3 text-sm w-1/5"),
						platformBadge(p.Platform),
					),
					Td(Class("px-4 py-3 text-sm text-slate-200 w-1/5 text-center"),
						Span(Class("text-emerald-400 font-medium"), g.Text(strconv.Itoa(p.InScopeCount))),
					),
					Td(Class("px-4 py-3 text-sm text-slate-200 w-1/5 text-center"),
						g.Text(strconv.Itoa(p.OutOfScopeCount)),
					),
				),
			)
		}
	}

	table := Div(Class("overflow-x-auto shadow-lg shadow-slate-950/50 rounded-lg border border-slate-700"),
		Table(Class("min-w-full divide-y divide-slate-700"),
			THead(Class("bg-slate-800"),
				tableHeaders,
			),
			TBody(Class("bg-slate-900 divide-y divide-slate-700"),
				g.Group(tableRows),
			),
		),
	)
	content = append(content, table)

	// Pagination bottom
	if result.TotalPages > 1 {
		content = append(content, Div(Class("mt-6 flex justify-center"),
			scopePagination(result.Page, result.TotalPages, search, sortBy, sortOrder, perPage, platform),
		))
	}

	return g.Group(content)
}

// scopePlatformFilterDropdown renders a multiselect dropdown for platform filtering.
func scopePlatformFilterDropdown(currentPlatform string, perPage int) g.Node {
	platforms := []struct{ Value, Label string }{
		{"hackerone", "HackerOne"},
		{"bugcrowd", "Bugcrowd"},
		{"intigriti", "Intigriti"},
		{"yeswehack", "YesWeHack"},
	}

	// Parse current selection
	selectedSet := make(map[string]bool)
	if currentPlatform != "" {
		for _, p := range strings.Split(currentPlatform, ",") {
			selectedSet[strings.TrimSpace(p)] = true
		}
	}
	allSelected := len(selectedSet) == 0

	// Build checkbox items
	var checkboxItems []g.Node
	for _, p := range platforms {
		isChecked := selectedSet[p.Value]
		attrs := []g.Node{
			Type("checkbox"),
			Name("platform"),
			Value(p.Value),
			Class("rounded border-slate-600 bg-slate-700 text-cyan-500 focus:ring-cyan-500 focus:ring-offset-0"),
		}
		if isChecked {
			attrs = append(attrs, Checked())
		}
		checkboxItems = append(checkboxItems,
			Label(Class("flex items-center gap-2 px-3 py-1.5 hover:bg-slate-700/50 cursor-pointer text-sm text-slate-300"),
				Input(attrs...),
				g.Text(p.Label),
			),
		)
	}

	// "All" button label
	buttonLabel := "All Platforms"
	if !allSelected {
		var names []string
		for _, p := range platforms {
			if selectedSet[p.Value] {
				names = append(names, p.Label)
			}
		}
		if len(names) <= 2 {
			buttonLabel = strings.Join(names, ", ")
		} else {
			buttonLabel = fmt.Sprintf("%d platforms", len(names))
		}
	}

	return Div(Class("relative mb-6"), ID("platform-filter"),
		// Toggle button
		Button(
			Type("button"),
			ID("platform-dropdown-btn"),
			Class("flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-lg bg-slate-800 text-slate-300 border border-slate-700 hover:bg-slate-700 hover:text-slate-200 transition-colors"),
			g.Attr("onclick", "document.getElementById('platform-dropdown-menu').classList.toggle('hidden')"),
			g.Text(buttonLabel),
			g.Raw(`<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>`),
		),
		// Dropdown panel
		Div(
			ID("platform-dropdown-menu"),
			Class("hidden absolute z-30 mt-1 w-56 bg-slate-800 border border-slate-700 rounded-lg shadow-xl py-1"),
			// "All" quick option
			Button(
				Type("button"),
				Class("w-full text-left px-3 py-1.5 text-sm text-cyan-400 hover:bg-slate-700/50 font-medium border-b border-slate-700 mb-1"),
				g.Attr("onclick", fmt.Sprintf(`
					document.querySelectorAll('#platform-dropdown-menu input[type=checkbox]').forEach(cb => cb.checked = false);
					applyPlatformFilter(%d);
				`, perPage)),
				g.Text("All Platforms"),
			),
			g.Group(checkboxItems),
			// Apply button
			Div(Class("px-3 pt-2 pb-1 border-t border-slate-700 mt-1"),
				Button(
					Type("button"),
					Class("w-full px-3 py-1.5 text-sm font-medium rounded bg-cyan-600 text-white hover:bg-cyan-500 transition-colors"),
					g.Attr("onclick", fmt.Sprintf("applyPlatformFilter(%d)", perPage)),
					g.Text("Apply"),
				),
			),
		),
		// Inline JS for platform filter
		Script(g.Raw(fmt.Sprintf(`
			function applyPlatformFilter(perPage) {
				var checked = [];
				document.querySelectorAll('#platform-dropdown-menu input[type=checkbox]:checked').forEach(function(cb) {
					checked.push(cb.value);
				});
				var url = '/scope?page=1&perPage=' + perPage;
				if (checked.length > 0) {
					url += '&platform=' + checked.join(',');
				}
				var container = document.getElementById('scope-table-container');
				if (container && typeof htmx !== 'undefined') {
					htmx.ajax('GET', url, {target: '#scope-table-container', pushUrl: true});
				} else {
					window.location.href = url;
				}
				document.getElementById('platform-dropdown-menu').classList.add('hidden');
			}
			// Close dropdown when clicking outside
			document.addEventListener('click', function(e) {
				var filter = document.getElementById('platform-filter');
				var menu = document.getElementById('platform-dropdown-menu');
				if (filter && menu && !filter.contains(e.target)) {
					menu.classList.add('hidden');
				}
			});
		`))),
	)
}

// scopeSearchBar renders the search form for the scope page.
func scopeSearchBar(search, sortBy, sortOrder string, perPage int, platform string) g.Node {
	return Div(Class("mb-6"),
		Form(Method("GET"), Action("/scope"),
			Class("flex flex-col sm:flex-row gap-2 items-stretch sm:items-center"),
			g.Attr("hx-get", "/scope"),
			g.Attr("hx-target", "#scope-table-container"),
			g.Attr("hx-push-url", "true"),
			g.Attr("hx-include", "[name='search'],[name='perPage'],[name='sortBy'],[name='sortOrder'],[name='platform']"),
			Input(
				Type("text"),
				Name("search"),
				Value(search),
				Placeholder("Search programs and assets..."),
				Class("flex-1 px-4 py-2 border border-slate-600 rounded-lg focus:ring-2 focus:ring-cyan-500 focus:border-transparent shadow-sm bg-slate-800 text-slate-200 placeholder-slate-500"),
			),
			Input(Type("hidden"), Name("perPage"), Value(strconv.Itoa(perPage))),
			Input(Type("hidden"), Name("sortBy"), Value(sortBy)),
			Input(Type("hidden"), Name("sortOrder"), Value(sortOrder)),
			Input(Type("hidden"), Name("platform"), Value(platform)),
			Input(Type("hidden"), Name("page"), Value("1")),
			Button(
				Type("submit"),
				Class("px-6 py-2 bg-cyan-600 text-white rounded-lg hover:bg-cyan-500 transition-colors shadow-sm"),
				g.Text("Search"),
			),
			g.If(search != "",
				A(
					Href(func() string {
						u := fmt.Sprintf("/scope?perPage=%d&sortBy=%s&sortOrder=%s", perPage, sortBy, sortOrder)
						if platform != "" {
							u += "&platform=" + platform
						}
						return u
					}()),
					Class("px-4 py-2 bg-slate-600 text-white rounded-lg hover:bg-slate-500 transition-colors text-center shadow-sm"),
					g.Text("Clear"),
				),
			),
		),
	)
}

// scopePerPageSelector renders the per-page dropdown for the scope page.
func scopePerPageSelector(search, sortBy, sortOrder string, currentPerPage int, platform string) g.Node {
	options := []g.Node{}
	for _, num := range []int{25, 50, 100, 250, 500} {
		opt := Option(Value(strconv.Itoa(num)), g.Text(fmt.Sprintf("%d items", num)))
		if num == currentPerPage {
			opt = Option(Value(strconv.Itoa(num)), g.Text(fmt.Sprintf("%d items", num)), Selected())
		}
		options = append(options, opt)
	}

	return Form(Method("GET"), Action("/scope"), Class("w-full sm:w-auto flex items-center justify-center sm:justify-start gap-1 sm:gap-2 text-sm"),
		Label(For("perPageSelect"), Class("text-slate-400 whitespace-nowrap"), g.Text("Items per page:")),
		Select(
			ID("perPageSelect"),
			Name("perPage"),
			g.Attr("onchange", "this.form.submit()"),
			Class("px-2 py-1 border border-slate-600 rounded-md shadow-sm focus:ring-cyan-500 focus:border-cyan-500 text-sm bg-slate-800 text-slate-200"),
			g.Group(options),
		),
		Input(Type("hidden"), Name("search"), Value(search)),
		Input(Type("hidden"), Name("sortBy"), Value(sortBy)),
		Input(Type("hidden"), Name("sortOrder"), Value(sortOrder)),
		Input(Type("hidden"), Name("platform"), Value(platform)),
		Input(Type("hidden"), Name("page"), Value("1")),
	)
}

// scopePagination creates pagination controls for the scope page with HTMX support.
func scopePagination(currentPage, totalPages int, search, sortBy, sortOrder string, perPage int, platform string) g.Node {
	var items []g.Node

	createPageLink := func(page int, text string, disabled, active bool) g.Node {
		href := fmt.Sprintf("/scope?page=%d&perPage=%d&sortBy=%s&sortOrder=%s", page, perPage, sortBy, sortOrder)
		if search != "" {
			href += "&search=" + url.QueryEscape(search)
		}
		if platform != "" {
			href += "&platform=" + platform
		}

		classes := "px-3 py-2 text-sm font-medium border rounded-md shadow-sm"
		if disabled {
			classes += " bg-slate-800 text-slate-500 cursor-not-allowed"
			return Span(Class(classes), g.Text(text))
		} else if active {
			classes += " bg-cyan-600 text-white border-cyan-600"
		} else {
			classes += " bg-slate-800 text-slate-300 border-slate-600 hover:bg-slate-700"
		}

		return A(
			Href(href),
			g.Attr("hx-get", href),
			g.Attr("hx-target", "#scope-table-container"),
			g.Attr("hx-push-url", "true"),
			Class(classes),
			g.Text(text),
		)
	}

	// Previous
	items = append(items, createPageLink(currentPage-1, "Previous", currentPage <= 1, false))

	// Page numbers
	start := max(1, currentPage-2)
	end := min(totalPages, currentPage+2)

	if start > 1 {
		items = append(items, createPageLink(1, "1", false, false))
		if start > 2 {
			items = append(items, Span(Class("px-3 py-2 text-sm text-slate-500"), g.Text("...")))
		}
	}
	for i := start; i <= end; i++ {
		items = append(items, createPageLink(i, strconv.Itoa(i), false, i == currentPage))
	}
	if end < totalPages {
		if end < totalPages-1 {
			items = append(items, Span(Class("px-3 py-2 text-sm text-slate-500"), g.Text("...")))
		}
		items = append(items, createPageLink(totalPages, strconv.Itoa(totalPages), false, false))
	}

	// Next
	items = append(items, createPageLink(currentPage+1, "Next", currentPage >= totalPages, false))

	return Div(Class("mt-6 flex justify-center"),
		Nav(Class("flex space-x-1"), g.Group(items)),
	)
}

// Helper functions for min/max
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// HTTP handler for the home page
func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	ctx := context.Background()
	totalPrograms, totalAssets, platformCount := 0, 0, 0
	stats, err := db.GetStats(ctx)
	if err == nil {
		platformCount = len(stats)
		for _, s := range stats {
			totalPrograms += s.ProgramCount
			totalAssets += s.InScopeCount + s.OutOfScopeCount
		}
	}

	PageLayout(
		"bbscope.com - A bug bounty scope aggregator",
		"Find and track bug bounty program scope data from HackerOne, Bugcrowd and other platforms. Search thousands of security targets and monitor scope changes.",
		Navbar(),
		MainContent(totalPrograms, totalAssets, platformCount),
		FooterEl(),
		"",
		false,
	).Render(w)
}

// HTTP handler for the /scope page
func scopeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/scope" {
		http.NotFound(w, r)
		return
	}

	ctx := context.Background()

	// Parse query parameters
	query := r.URL.Query()
	search := strings.TrimSpace(query.Get("search"))
	sortBy := strings.ToLower(strings.TrimSpace(query.Get("sortBy")))
	sortOrder := strings.ToLower(strings.TrimSpace(query.Get("sortOrder")))
	platformFilter := strings.ToLower(strings.TrimSpace(query.Get("platform")))

	page := 1
	if p, err := strconv.Atoi(query.Get("page")); err == nil && p > 0 {
		page = p
	}

	currentPerPage := 50
	allowedPerPages := []int{25, 50, 100, 250, 500}
	if p, err := strconv.Atoi(query.Get("perPage")); err == nil {
		for _, allowed := range allowedPerPages {
			if p == allowed {
				currentPerPage = p
				break
			}
		}
	}

	// Validate sortBy
	validSortCols := map[string]bool{"handle": true, "platform": true, "in_scope_count": true, "out_of_scope_count": true}
	if !validSortCols[sortBy] {
		sortBy = "handle"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "asc"
	}

	// Parse comma-separated platforms into a slice
	var platformSlice []string
	if platformFilter != "" {
		for _, p := range strings.Split(platformFilter, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				platformSlice = append(platformSlice, p)
			}
		}
	}

	// Query DB with SQL-level pagination
	result, err := db.ListProgramsPaginated(ctx, storage.ProgramListOptions{
		Platforms: platformSlice,
		Search:    search,
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Page:      page,
		PerPage:   currentPerPage,
	})

	var loadErr error
	if err != nil {
		loadErr = err
		log.Printf("Error listing programs: %v", err)
		result = &storage.ProgramListResult{
			Programs:   nil,
			TotalCount: 0,
			Page:       page,
			PerPage:    currentPerPage,
			TotalPages: 1,
		}
	}

	// Clamp page
	if page > result.TotalPages {
		page = result.TotalPages
	}
	if page < 1 {
		page = 1
	}

	// Check if this is an HTMX request
	isHTMX := r.Header.Get("HX-Request") == "true"

	if isHTMX {
		// Return only the table fragment for HTMX swap
		ScopeTableFragment(result, loadErr, search, sortBy, sortOrder, currentPerPage, platformFilter).Render(w)
		return
	}

	// Full page render
	canonicalURL := fmt.Sprintf("/scope?page=%d", page)
	pageTitle := fmt.Sprintf("Scope data - bbscope.com (Page %d)", page)
	pageDescription := "Browse and download bug bounty scope data from all bug bounty platforms. Find in-scope websites from HackerOne, Bugcrowd, Intigriti and YesWeHack."
	if page > 1 {
		pageDescription = fmt.Sprintf("%s (Page %d)", pageDescription, page)
	}

	PageLayout(
		pageTitle,
		pageDescription,
		Navbar(),
		ScopeContent(result, loadErr, search, sortBy, sortOrder, currentPerPage, platformFilter),
		FooterEl(),
		canonicalURL,
		page > 1,
	).Render(w)
}

// HTTP handler for /robots.txt
func robotsTxtHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/robots.txt" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "User-agent: *")
	fmt.Fprintln(w, "Allow: /")
	fmt.Fprintf(w, "Sitemap: https://%s/sitemap.xml\n", serverDomain)
}

// Helper to make change type more readable for display
func formatDisplayChangeType(changeType string) string {
	switch changeType {
	case "program_added":
		return "Program Added"
	case "program_removed":
		return "Program Removed"
	case "asset_added":
		return "Asset Added"
	case "asset_removed":
		return "Asset Removed"
	default:
		return strings.ReplaceAll(strings.Title(strings.ReplaceAll(changeType, "_", " ")), " ", "")
	}
}


// truncateMiddle shortens a string by replacing the middle part with ellipsis
// if it exceeds maxLength.
func truncateMiddle(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	if maxLength <= 3 { // Cannot meaningfully truncate to less than "..."
		// Return the beginning of the string if maxLength is too small for ellipsis
		if maxLength < 0 {
			maxLength = 0
		}
		return s[:maxLength]
	}
	ellipsis := "..."
	// Ensure non-negative lengths for slices
	firstHalfLen := (maxLength - len(ellipsis)) / 2
	if firstHalfLen < 0 {
		firstHalfLen = 0
	}
	secondHalfLen := maxLength - len(ellipsis) - firstHalfLen
	if secondHalfLen < 0 {
		secondHalfLen = 0
	}
	if len(s) < firstHalfLen+secondHalfLen { // Should not happen if len(s) > maxLength
		return s // Or handle as error/edge case
	}

	return s[:firstHalfLen] + ellipsis + s[len(s)-secondHalfLen:]
}

func Run(cfg ServerConfig) error {
	var err error
	db, err = storage.Open(cfg.DBUrl)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	serverDomain = cfg.Domain

	if cfg.PollInterval > 0 {
		go startBackgroundPoller(cfg)
	}

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/scope", scopeHandler)
	http.HandleFunc("/program/", programDetailHandler)
	http.HandleFunc("/updates", updatesHandler)
	http.HandleFunc("/docs", docsHandler)
	http.HandleFunc("/stats", statsHandler)
	http.HandleFunc("/contact", contactHandler)
	http.HandleFunc("/robots.txt", robotsTxtHandler)
	http.HandleFunc("/sitemap.xml", sitemapHandler)

	// Serve static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("website/static"))))

	listenAddr := cfg.ListenAddr
	if cfg.DevMode && listenAddr == ":8080" {
		listenAddr = "localhost:7000"
	}
	if cfg.DevMode {
		log.Printf("Starting server in development mode on http://%s", listenAddr)
	} else {
		log.Printf("Starting server on %s (domain: %s)", listenAddr, serverDomain)
	}

	return http.ListenAndServe(listenAddr, nil)
}

// UpdatesContent renders the main content for the /updates page.
func UpdatesContent(updates []UpdateEntry, currentPage, totalPages int, currentPerPage int, currentSearch string, isGoogleBot bool, currentPlatform string) g.Node {
	pageContent := []g.Node{
		H1(Class("text-3xl font-bold text-slate-100 mb-4"), g.Text("Scope Updates")),
		P(Class("text-slate-400 mb-6"), g.Text("Recent changes to bug bounty program scopes.")),
		platformFilterTabs("/updates", currentPlatform, fmt.Sprintf("&perPage=%d", currentPerPage)),
	}

	// Search controls
	// Original Form class was: "flex flex-col sm:flex-row justify-between items-center mb-4 gap-4"
	// We'll adopt classes similar to the /scope page's search form for better responsive behavior.
	controlsHeader := Form(Method("GET"), Action("/updates"), Class("flex flex-col sm:flex-row gap-2 items-stretch sm:items-center mb-4"), // MODIFIED CLASS
		Input(
			Type("text"),
			Name("search"),
			Placeholder("Search updates..."),
			Value(currentSearch),
			Class("flex-1 px-4 py-2 border border-slate-600 rounded-lg focus:ring-2 focus:ring-cyan-500 focus:border-transparent shadow-sm bg-slate-800 text-slate-200 placeholder-slate-500"), // flex-1 allows input to grow in flex-row
		),
		Button(
			Type("submit"),
			Class("px-6 py-2 bg-cyan-600 text-white rounded-lg hover:bg-cyan-500 transition-colors shadow-sm"), // items-stretch on Form will handle width on mobile
			g.Text("Search"),
		),
		Input(Type("hidden"), Name("perPage"), Value(strconv.Itoa(currentPerPage))),
		Input(Type("hidden"), Name("platform"), Value(currentPlatform)),
	)
	pageContent = append(pageContent, controlsHeader)

	var tableRows []g.Node
	if len(updates) == 0 {
		noResultsMsg := "No updates to display."
		if currentSearch != "" {
			noResultsMsg = fmt.Sprintf("No updates found for search '%s'.", currentSearch) // Updated message
		}
		tableRows = append(tableRows,
			Tr(Td(ColSpan("6"), Class("text-center py-10 text-slate-400"), g.Text(noResultsMsg))),
		)
	} else {
		for i, entry := range updates {
			rowID := fmt.Sprintf("update-row-%d-%d", currentPage, i)
			detailsID := fmt.Sprintf("update-details-%d-%d", currentPage, i)
			iconID := fmt.Sprintf("update-icon-%d-%d", currentPage, i)

			isProgramLevelChange := entry.Type == "program_added" || entry.Type == "program_removed"

			var assetDisplay g.Node
			var firstCell g.Node

			if isProgramLevelChange {
				expanderIcon := "+"
				if isGoogleBot {
					expanderIcon = "-"
				}
				// For program_added/removed, show a summary and make it expandable
				firstCell = Td(Class("px-2 py-3 text-center"),
					Span(ID(iconID), Class("expand-icon font-mono text-lg text-cyan-400 hover:text-cyan-300"), g.Text(expanderIcon)),
				)
				assetDisplay = g.Text(formatDisplayChangeType(entry.Type)) // "Program Added" or "Program Removed"
			} else {
				// For asset_added/removed, show the asset directly with type prefix
				firstCell = Td(Class("px-2 py-3")) // Empty cell for alignment

				changeTypeDisplay := formatDisplayChangeType(entry.Type) // "Asset Added" or "Asset Removed"
				assetCategory := entry.Asset.Category
				if assetCategory == "" {
					assetCategory = "N/A"
				}
				// Construct the display string: "Asset Added: CATEGORY: VALUE"
				assetDisplay = Div(
					Strong(Class("text-slate-300"), g.Text(changeTypeDisplay+": "+strings.ToUpper(assetCategory)+": ")),
					Span(Class("text-slate-200 break-all"), g.Text(entry.Asset.Value)),
				)
			}

			rowClasses := "border-b border-slate-700/50 hover:bg-slate-800/70 transition-colors"
			if i%2 == 1 {
				rowClasses += " bg-slate-800/30"
			}
			var rowOnClick g.Node // Changed from g.AttrBuilder
			if isProgramLevelChange {
				rowClasses += " cursor-pointer"
				rowOnClick = g.Attr("onclick", fmt.Sprintf("toggleScopeDetails('%s', '%s')", detailsID, iconID))
			}

			tableRows = append(tableRows,
				Tr(Class(rowClasses), rowOnClick, ID(rowID),
					firstCell, // Expander or empty cell
					Td(Class("px-4 py-3 text-sm"), assetDisplay),
					Td(Class("px-4 py-3 text-sm text-slate-300"), g.Text(entry.ScopeType)),
					Td(Class("px-4 py-3 text-sm"),
						A(Href(entry.ProgramURL), Target("_blank"), Rel("noopener noreferrer"),
							Class("text-cyan-400 hover:text-cyan-300 hover:underline"),
							g.Attr("title", entry.ProgramURL),
							g.If(isProgramLevelChange, g.Attr("onclick", "event.stopPropagation();")),
							g.Text(truncateMiddle(entry.ProgramURL, 50)),
						),
					),
					Td(Class("px-4 py-3 text-sm text-slate-300"), g.Text(entry.Platform)),
					Td(Class("px-4 py-3 text-sm text-slate-300"), g.Text(entry.Timestamp.Format("2006-01-02 15:04"))),
				),
			)

			if isProgramLevelChange {
				detailsRowClass := "hidden bg-slate-800/50"
				if isGoogleBot {
					detailsRowClass = "bg-slate-800/50" // Not hidden
				}

				var detailsRowContent g.Node
				if len(entry.AssociatedAssets) > 0 {
					detailsContentNodes := []g.Node{}
					detailsContentNodes = append(detailsContentNodes, H4(Class("font-semibold text-slate-300 mb-1 text-base pt-2"), g.Text("Associated Assets:")))
					assetListItems := []g.Node{}
					for _, asset := range entry.AssociatedAssets {
						assetText := asset.Value
						if asset.Category != "" {
							assetText = fmt.Sprintf("%s: %s", strings.ToUpper(asset.Category), asset.Value)
						}
						assetListItems = append(assetListItems, Li(Class("ml-4 text-sm text-slate-400 py-0.5"), g.Text(assetText)))
					}
					detailsContentNodes = append(detailsContentNodes, Ul(Class("list-disc list-inside mb-2"), g.Group(assetListItems)))
					detailsRowContent = g.Group(detailsContentNodes)
				} else {
					// Case where a program was added/removed but somehow no assets were logged with it (edge case)
					detailsRowContent = P(Class("text-sm text-slate-400 py-2"), g.Text("No specific asset details were logged for this program change."))
				}

				detailsRow := Tr(
					ID(detailsID),
					Class(detailsRowClass), // Use computed class
					Td(
						ColSpan("6"), // Span all 6 columns
						Class("p-0"),
						Div(Class("p-3 border-t border-b border-slate-700"),
							Div(Class("p-2 rounded bg-slate-900 shadow-inner"),
								detailsRowContent,
							),
						),
					),
				)
				tableRows = append(tableRows, detailsRow)
			}
		}
	}

	table := Div(Class("overflow-x-auto shadow-lg shadow-slate-950/50 rounded-lg mt-0 border border-slate-700"),
		Table(Class("min-w-full divide-y divide-slate-700 table-fixed"),
			THead(Class("bg-slate-800"),
				Tr(
					Th(Class("px-2 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider w-10")), // Expander column
					Th(Class("px-4 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider w-2/6"), g.Text("Asset / Change")),
					Th(Class("px-4 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider w-1/12"), g.Text("Scope Type")),
					Th(Class("px-4 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider w-2/6"), g.Text("Program URL")),
					Th(Class("px-4 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider w-1/12"), g.Text("Platform")),
					Th(Class("px-4 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider w-1/6"), g.Text("Timestamp")),
				),
			),
			TBody(Class("bg-slate-900 divide-y divide-slate-700"),
				g.Group(tableRows),
			),
		),
	)
	pageContent = append(pageContent, table)

	// Pagination
	if totalPages > 1 {
		paginationBottom := createUpdatesPagePagination(currentPage, totalPages, currentPerPage, currentPlatform)
		pageContent = append(pageContent, Div(Class("mt-6 flex justify-center"), paginationBottom))
	}

	return Main(Class("container mx-auto mt-8 mb-16 p-4"),
		Section(Class("bg-slate-900/50 border border-slate-800 rounded-lg shadow-xl p-6 md:p-8 lg:p-12"),
			g.Group(pageContent),
		),
	)
}

// createUpdatesPagePagination creates pagination controls for the updates page
func createUpdatesPagePagination(currentPage, totalPages int, perPage int, platform string) g.Node {
	var paginationItems []g.Node

	// Helper function to create pagination link
	createPageLink := func(page int, text string, disabled bool, active bool) g.Node {
		href := fmt.Sprintf("/updates?page=%d&perPage=%d", page, perPage)
		if platform != "" {
			href += "&platform=" + platform
		}

		classes := "px-3 py-2 text-sm font-medium border rounded-md shadow-sm"
		if disabled {
			classes += " bg-slate-800 text-slate-500 cursor-not-allowed"
			return Span(Class(classes), g.Text(text))
		} else if active {
			classes += " bg-cyan-600 text-white border-cyan-600"
		} else {
			classes += " bg-slate-800 text-slate-300 border-slate-600 hover:bg-slate-700"
		}

		return A(Href(href), Class(classes), g.Text(text))
	}

	// Previous button
	paginationItems = append(paginationItems,
		createPageLink(currentPage-1, "Previous", currentPage <= 1, false),
	)

	// Page numbers
	start := max(1, currentPage-2)
	end := min(totalPages, currentPage+2)

	if start > 1 {
		paginationItems = append(paginationItems, createPageLink(1, "1", false, false))
		if start > 2 {
			paginationItems = append(paginationItems,
				Span(Class("px-3 py-2 text-sm text-slate-500"), g.Text("...")),
			)
		}
	}

	for i := start; i <= end; i++ {
		paginationItems = append(paginationItems,
			createPageLink(i, strconv.Itoa(i), false, i == currentPage),
		)
	}

	if end < totalPages {
		if end < totalPages-1 {
			paginationItems = append(paginationItems,
				Span(Class("px-3 py-2 text-sm text-slate-500"), g.Text("...")),
			)
		}
		paginationItems = append(paginationItems,
			createPageLink(totalPages, strconv.Itoa(totalPages), false, false),
		)
	}

	// Next button
	paginationItems = append(paginationItems,
		createPageLink(currentPage+1, "Next", currentPage >= totalPages, false),
	)

	return Div(Class("mt-6 flex justify-center"),
		Nav(Class("flex space-x-1"),
			g.Group(paginationItems),
		),
	)
}

// updatesHandler handles requests for the /updates page.
func updatesHandler(w http.ResponseWriter, r *http.Request) {
	// Check if the user agent is Googlebot
	userAgent := r.Header.Get("User-Agent")
	isGoogleBot := strings.Contains(strings.ToLower(userAgent), "googlebot")

	allUpdates, err := loadUpdatesFromDB()
	if err != nil {
		log.Printf("Error loading updates data: %v", err)
		http.Error(w, "Could not load updates data", http.StatusInternalServerError)
		return
	}

	// Preprocessing logic
	assetEventsByProgramAndTimestamp := make(map[string]map[time.Time][]UpdateEntryAsset)
	programLevelEventTimestamps := make(map[string]map[time.Time]bool) // To track if a program-level event exists for a given program/timestamp

	for _, entry := range allUpdates {
		if entry.Type == "asset_added" || entry.Type == "asset_removed" {
			if _, ok := assetEventsByProgramAndTimestamp[entry.ProgramURL]; !ok {
				assetEventsByProgramAndTimestamp[entry.ProgramURL] = make(map[time.Time][]UpdateEntryAsset)
			}
			assetEventsByProgramAndTimestamp[entry.ProgramURL][entry.Timestamp] = append(
				assetEventsByProgramAndTimestamp[entry.ProgramURL][entry.Timestamp],
				entry.Asset,
			)
		} else if entry.Type == "program_added" || entry.Type == "program_removed" {
			if _, ok := programLevelEventTimestamps[entry.ProgramURL]; !ok {
				programLevelEventTimestamps[entry.ProgramURL] = make(map[time.Time]bool)
			}
			programLevelEventTimestamps[entry.ProgramURL][entry.Timestamp] = true
		}
	}

	var processedUpdates []UpdateEntry
	for _, entry := range allUpdates {
		if entry.Type == "program_added" || entry.Type == "program_removed" {
			displayEntry := entry // Create a copy
			if assets, ok := assetEventsByProgramAndTimestamp[entry.ProgramURL][entry.Timestamp]; ok {
				displayEntry.AssociatedAssets = assets
			}
			processedUpdates = append(processedUpdates, displayEntry)
		} else if entry.Type == "asset_added" || entry.Type == "asset_removed" {
			// Check if this asset event is part of a program-level event
			isStandaloneAssetEvent := true
			if programTimestamps, ok := programLevelEventTimestamps[entry.ProgramURL]; ok {
				if _, programEventExists := programTimestamps[entry.Timestamp]; programEventExists {
					isStandaloneAssetEvent = false // This asset change is covered by a program_added/removed event
				}
			}
			if isStandaloneAssetEvent {
				processedUpdates = append(processedUpdates, entry)
			}
		}
	}
	// Sort the final list by timestamp descending
	sort.SliceStable(processedUpdates, func(i, j int) bool {
		return processedUpdates[i].Timestamp.After(processedUpdates[j].Timestamp)
	})

	// Get query parameters
	query := r.URL.Query()
	pageStr := query.Get("page")
	searchQuery := strings.TrimSpace(query.Get("search"))
	platformFilter := strings.ToLower(strings.TrimSpace(query.Get("platform")))
	perPageStr := query.Get("perPage")

	// Apply search filter if searchQuery is present
	if searchQuery != "" {
		searchLower := strings.ToLower(searchQuery)
		var filteredUpdates []UpdateEntry
		for _, entry := range processedUpdates {
			match := false
			// Check common fields
			if strings.Contains(strings.ToLower(entry.Type), searchLower) ||
				strings.Contains(strings.ToLower(entry.ProgramURL), searchLower) ||
				strings.Contains(strings.ToLower(entry.Platform), searchLower) {
				match = true
			}

			if !match { // If not matched yet, check type-specific fields
				if entry.Type == "program_added" || entry.Type == "program_removed" {
					for _, asset := range entry.AssociatedAssets {
						if strings.Contains(strings.ToLower(asset.Category), searchLower) ||
							strings.Contains(strings.ToLower(asset.Value), searchLower) {
							match = true
							break
						}
					}
				} else { // asset_added or asset_removed (standalone)
					if strings.Contains(strings.ToLower(entry.ScopeType), searchLower) ||
						strings.Contains(strings.ToLower(entry.Asset.Category), searchLower) ||
						strings.Contains(strings.ToLower(entry.Asset.Value), searchLower) {
						match = true
					}
				}
			}

			if match {
				filteredUpdates = append(filteredUpdates, entry)
			}

		}
		processedUpdates = filteredUpdates
	}

	// Platform filter
	if platformFilter != "" {
		var platformFiltered []UpdateEntry
		for _, entry := range processedUpdates {
			if strings.ToLower(entry.Platform) == platformFilter {
				platformFiltered = append(platformFiltered, entry)
			}
		}
		processedUpdates = platformFiltered
	}

	// Pagination logic (using processedUpdates)
	// Use existing query and pageStr variables already declared above
	// pageStr is re-read here because the search filtering might have changed the effective page number
	currentPage := 1 // Reset to 1 for updates pagination start
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			currentPage = p
		}
	}

	currentPerPage := 25 // Default items per page
	allowedPerPages := []int{25, 50, 100, 250}
	if perPageStr != "" {
		if p, err := strconv.Atoi(perPageStr); err == nil {
			for _, allowed := range allowedPerPages {
				if p == allowed {
					currentPerPage = p
					break
				}
			}
		}
	}

	totalPages := 0
	if len(processedUpdates) > 0 {
		totalPages = (len(processedUpdates) + currentPerPage - 1) / currentPerPage
		if totalPages == 0 && len(processedUpdates) > 0 { // Ensure at least one page if there are items
			totalPages = 1
		}
	}

	// Ensure currentPage is not out of bounds
	if currentPage > totalPages && totalPages > 0 {
		currentPage = totalPages
	}
	if currentPage < 1 {
		currentPage = 1
	}

	startIndex := (currentPage - 1) * currentPerPage
	endIndex := startIndex + currentPerPage
	if endIndex > len(processedUpdates) {
		endIndex = len(processedUpdates)
	}

	paginatedUpdates := processedUpdates[startIndex:endIndex]

	// Canonical URL for updates page (only page parameter)
	updatesCanonicalURL := fmt.Sprintf("/updates?page=%d", currentPage)

	// Determine page title for /updates based on current page
	updatesPageTitle := fmt.Sprintf("Scope Updates - bbscope.com (Page %d)", currentPage)

	// Determine page description for /updates based on current page
	updatesPageDescription := "Recent changes to bug bounty program scopes from HackerOne, Bugcrowd, Intigriti and YesWeHack."
	if currentPage > 1 {
		updatesPageDescription = fmt.Sprintf("%s (Page %d)", updatesPageDescription, currentPage)
	}

	PageLayout(
		updatesPageTitle,
		updatesPageDescription,
		Navbar(),
		UpdatesContent(paginatedUpdates, currentPage, totalPages, currentPerPage, searchQuery, isGoogleBot, platformFilter),
		FooterEl(),
		updatesCanonicalURL,
		currentPage > 1,
	).Render(w)
}
