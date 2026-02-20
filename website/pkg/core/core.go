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

// ScopeEntry holds data for a single scope item
type ScopeEntry struct {
	Element    string
	ProgramURL string
	Category   string
}

// ProgramSummaryEntry holds aggregated data for a program for the compact view
type ProgramSummaryEntry struct {
	ProgramURL      string
	InScopeCount    int
	OutOfScopeCount int
	Platform        string
	Elements        []string // New: Store all elements for comprehensive search
}

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

// Sort constants
const (
	SortByElement    = "element"
	SortByCategory   = "category"
	SortByProgramURL = "programurl" // Used by both views
	SortOrderAsc     = "asc"
	SortOrderDesc    = "desc"

	// New Sort constants for compact view
	SortByInScopeCount    = "inscapecount"
	SortByOutOfScopeCount = "outscopecount"
	SortByPlatformName    = "platform"
)

// loadScopeFromDB loads all scope entries from the database.
func loadScopeFromDB() ([]ScopeEntry, error) {
	ctx := context.Background()
	entries, err := db.ListEntries(ctx, storage.ListOptions{IncludeOOS: true})
	if err != nil {
		return nil, fmt.Errorf("failed to load scope from database: %w", err)
	}

	var result []ScopeEntry
	for _, e := range entries {
		element := strings.TrimSpace(e.TargetNormalized)
		if element == "" {
			element = strings.TrimSpace(e.TargetRaw)
		}
		category := strings.ToUpper(scope.NormalizeCategory(e.Category))
		programURL := strings.ReplaceAll(e.ProgramURL, "api.yeswehack.com", "yeswehack.com")

		if !e.InScope {
			if strings.Contains(element, "██") {
				element = "REDACTED (OOS)"
			} else {
				element += " (OOS)"
			}
		} else {
			if strings.Contains(element, "██") {
				element = "REDACTED"
			}
		}

		result = append(result, ScopeEntry{
			Element:    element,
			ProgramURL: programURL,
			Category:   category,
		})
	}
	return result, nil
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

// sortEntries sorts a slice of ScopeEntry based on the sortBy field and sortOrder.
// It sorts the slice in place.
// If sortOrder is an empty string, no sorting is performed for that column.
func sortEntries(entries []ScopeEntry, sortBy, sortOrder string) {
	if sortOrder == "" { // If sortOrder is empty, this column is not actively sorted.
		return // Entries remain in their current order (e.g., natural load order or previously sorted).
	}

	// If sortBy is empty but sortOrder is 'asc' or 'desc' (which implies an active sort),
	// default sortBy to Element. This handles direct URL manipulation like ?sortOrder=asc.
	if sortBy == "" && (sortOrder == SortOrderAsc || sortOrder == SortOrderDesc) {
		sortBy = SortByElement
	}

	sort.SliceStable(entries, func(i, j int) bool {
		var less bool
		// Ensure case-insensitive comparison for strings
		elemI := strings.ToLower(entries[i].Element)
		elemJ := strings.ToLower(entries[j].Element)
		catI := strings.ToLower(entries[i].Category)
		catJ := strings.ToLower(entries[j].Category)
		urlI := strings.ToLower(entries[i].ProgramURL)
		urlJ := strings.ToLower(entries[j].ProgramURL)

		switch sortBy {
		case SortByElement:
			less = elemI < elemJ
		case SortByCategory:
			less = catI < catJ
		case SortByProgramURL:
			less = urlI < urlJ
		default:
			// If sortBy is an unrecognized value, but sortOrder is present (asc/desc),
			// default to sorting by element.
			less = elemI < elemJ
		}

		if sortOrder == SortOrderDesc {
			return !less
		}
		// Assumes SortOrderAsc or any other non-empty, non-desc value means ascending.
		// scopeHandler normalizes sortOrder, so this primarily covers SortOrderAsc.
		return less
	})
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
				Link(Rel("stylesheet"), Href("https://cdnjs.cloudflare.com/ajax/libs/tailwindcss/2.2.19/tailwind.min.css")), // For production, consider self-hosting or a modern CDN
				Link(Rel("stylesheet"), Href("https://cdn.jsdelivr.net/npm/@tailwindcss/typography@0.5.x/dist/typography.min.css")),

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
					/* Custom scrollbar styling */
					/* For Webkit browsers */
					::-webkit-scrollbar {
						width: 8px;
						height: 8px;
					}
					::-webkit-scrollbar-track {
						background: #f1f1f1;
						border-radius: 10px;
					}
					::-webkit-scrollbar-thumb {
						background: #888;
						border-radius: 10px;
					}
					::-webkit-scrollbar-thumb:hover {
						background: #555;
					}
					.table-fixed { table-layout: fixed; } /* Ensure table layout respects column widths */
				`)),
			),
			Body(Class("bg-gray-100 font-sans leading-normal tracking-normal flex flex-col min-h-screen"),
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
	return Nav(Class("bg-gray-800 text-white p-4 shadow-md sticky top-0 z-50"),
		Div(Class("container mx-auto flex justify-between items-center"),
			// Logo/Site Name
			A(Href("/"), Class("text-2xl font-bold hover:text-gray-300 transition-colors"), g.Text("bbscope.com")),

			// Mobile Menu Button (Hamburger)
			Div(Class("md:hidden"), // Visible on small screens, hidden on medium and up
				Button(
					ID("mobile-menu-button"), // ID for JavaScript
					Type("button"),
					Class("inline-flex items-center justify-center p-2 rounded-md text-gray-400 hover:text-white hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-white"),
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
				Class("hidden md:flex md:items-center md:space-x-1 w-full md:w-auto absolute md:relative top-16 left-0 md:top-auto md:left-auto bg-gray-800 md:bg-transparent shadow-lg md:shadow-none rounded-b-md md:rounded-none py-2 md:py-0"),
				A(Href("/"), Class("block text-center md:inline-block hover:bg-gray-700 md:hover:bg-transparent hover:text-white md:hover:text-gray-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("Home")),
				A(Href("/scope"), Class("block text-center md:inline-block hover:bg-gray-700 md:hover:bg-transparent hover:text-white md:hover:text-gray-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("Scope")),     // New Scope Link
				A(Href("/updates"), Class("block text-center md:inline-block hover:bg-gray-700 md:hover:bg-transparent hover:text-white md:hover:text-gray-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("Updates")), // New Updates Link
				A(Href("/stats"), Class("block text-center md:inline-block hover:bg-gray-700 md:hover:bg-transparent hover:text-white md:hover:text-gray-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("Stats")),     // New Stats Link
				A(Href("/docs"), Class("block text-center md:inline-block hover:bg-gray-700 md:hover:bg-transparent hover:text-white md:hover:text-gray-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("Docs")),
				A(Href("https://github.com/sw33tLie/bbscope"), Target("_blank"), Rel("noopener noreferrer"), Class("block text-center md:inline-block hover:bg-gray-700 md:hover:bg-transparent hover:text-white md:hover:text-gray-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("GitHub")),
				A(Href("/contact"), Class("block text-center md:inline-block hover:bg-gray-700 md:hover:bg-transparent hover:text-white md:hover:text-gray-300 transition-colors px-3 py-2 rounded-md text-sm font-medium"), g.Text("Contact")),
			),
		),
	)
}

// MainContent component for the landing page
func MainContent() g.Node {
	newHeroSection := Section(
		Div(Class("relative items-center w-full px-5 py-12 mx-auto md:px-12 lg:px-16 max-w-7xl lg:py-24"),
			Div(Class("flex w-full mx-auto text-left"),
				Div(Class("relative inline-flex items-center mx-auto align-middle"),
					Div(Class("text-center"),
						Img( // Added logo here
							Class("block mx-auto mb-6 h-24 w-auto"),
							Src("/static/images/bbscope-logo.svg"),
							Alt("bbscope.com logo"),
						),
						H1(Class("max-w-5xl text-2xl font-bold leading-none tracking-tighter text-neutral-600 md:text-5xl lg:text-6xl lg:max-w-7xl"),
							g.Text("Bug Bounty Scope Data Aggregator"),
						),
						P(Class("max-w-xl mx-auto mt-8 text-base leading-relaxed text-gray-500"), g.Raw("This website collects public bug bounty targets fetched with <a href='https://github.com/sw33tLie/bbscope' class='text-blue-600 hover:text-blue-800 underline'>bbscope cli</a>.<br>We have a few extra tools too!")),
						Div(Class("flex justify-center items-center w-full max-w-2xl gap-2 mx-auto mt-6"), // Added items-center for button alignment
							Div(Class("mt-3 rounded-lg sm:mt-0"),
								A(Href("/scope"), Class("px-5 py-4 text-base font-medium text-center text-white transition duration-500 ease-in-out transform bg-blue-600 lg:px-10 rounded-xl hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"), g.Text("View scope")),
							),
							Div(Class("mt-3 rounded-lg sm:mt-0 sm:ml-3"),
								A(Href("/updates"), Class("items-center block px-5 lg:px-10 py-3.5 text-base font-medium text-center text-blue-600 transition duration-500 ease-in-out transform border-2 border-white shadow-md rounded-xl focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-500"), g.Text("Latest changes")),
							),
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
			H2(Class("text-3xl font-bold text-center text-gray-800 mb-10"), g.Text("Use cases")),
			Div(Class("grid md:grid-cols-3 gap-8"),
				featureCard("Quick Scope", "You can quickly view and download bug bounty scope and use it for your own purposes"),
				featureCard("Track changes", "We track all scope changes. Hunt on fresh targets!"),
				featureCard("Stats", "Get platform insights and statistics for bug bounty programs."),
			),
		),

		// Call to Action Section
		Section(Class("py-12 bg-gray-200 rounded-lg"),
			Div(Class("container mx-auto text-center px-4"),
				H2(Class("text-3xl font-bold text-gray-800 mb-4"), g.Text("Want to help?")),
				P(Class("text-gray-600 mb-8 max-w-xl mx-auto"), g.Text("This tool is powered by the bbscope CLI tool. Pull requests are welcome!")),
				A(Href("https://github.com/sw33tLie/bbscope"), Target("_blank"), Rel("noopener noreferrer"), Class("bg-green-500 hover:bg-green-600 text-white font-bold py-3 px-8 rounded-lg text-lg transition duration-300 ease-in-out transform hover:scale-105"),
					g.Text("Go to GitHub"),
				),
			),
		),
	)
}

// Helper function for feature cards
func featureCard(title, description string) g.Node {
	return Div(Class("md:bg-white md:shadow-lg md:rounded-lg md:p-6 transform md:hover:shadow-xl transition-shadow duration-300"),
		H3(Class("text-xl font-semibold text-gray-700 mb-3"), g.Text(title)),
		P(Class("text-gray-600 text-sm"), g.Text(description)),
	)
}

// FooterEl component (using El suffix to avoid conflict with html.Footer)
func FooterEl() g.Node {
	currentYear := time.Now().Year()
	return Footer(Class("bg-gray-800 text-gray-300 text-center p-6 mt-auto"),
		Div(Class("container mx-auto"),
			P(g.Raw(fmt.Sprintf("© %d bbscope.com. All rights reserved. Made by <a href='https://x.com/sw33tLie' class='hover:text-blue-800 underline'>sw33tLie</a>", currentYear))),
			Div(Class("mt-2"),
				A(Href("#privacy"), Class("text-gray-400 hover:text-white mx-2 text-sm"), g.Text("Privacy Policy")),
				A(Href("#terms"), Class("text-gray-400 hover:text-white mx-2 text-sm"), g.Text("Terms of Service")),
			),
		),
	)
}

// filterEntries filters entries based on search query and OOS visibility (for detailed view)
func filterEntries(entries []ScopeEntry, search string, hideOOS bool) []ScopeEntry {
	if search == "" && !hideOOS { // If no search and not hiding OOS, return all
		return entries
	}

	searchLower := strings.ToLower(strings.TrimSpace(search))
	var filtered []ScopeEntry

	for _, entry := range entries {
		// OOS check
		if hideOOS && strings.Contains(entry.Element, "(OOS)") {
			continue // Skip OOS items if hideOOS is true
		}

		// Search check (only if search term is present)
		if searchLower != "" {
			if !strings.Contains(strings.ToLower(entry.Element), searchLower) &&
				!strings.Contains(strings.ToLower(entry.Category), searchLower) &&
				!strings.Contains(strings.ToLower(entry.ProgramURL), searchLower) {
				continue // Skip if search term doesn't match
			}
		}
		// If we reach here, the entry passes OOS filter (if active) and search filter (if active)
		filtered = append(filtered, entry)
	}

	return filtered
}

// paginateEntries returns a slice of entries for the given page (for detailed view)
func paginateEntries(entries []ScopeEntry, page, perPage int) []ScopeEntry {
	start := (page - 1) * perPage
	end := start + perPage

	if start >= len(entries) {
		return []ScopeEntry{}
	}

	if end > len(entries) {
		end = len(entries)
	}

	return entries[start:end]
}

// paginateProgramSummaries returns a slice of program summaries for the given page (for compact view)
func paginateProgramSummaries(summaries []ProgramSummaryEntry, page, perPage int) []ProgramSummaryEntry {
	start := (page - 1) * perPage
	end := start + perPage

	if start >= len(summaries) {
		return []ProgramSummaryEntry{}
	}

	if end > len(summaries) {
		end = len(summaries)
	}

	return summaries[start:end]
}

// getPlatformFromURL determines the platform from the program URL.
func getPlatformFromURL(programURL string) string {
	lowerURL := strings.ToLower(programURL)
	if strings.Contains(lowerURL, "hackerone.com") {
		return "HackerOne"
	}
	if strings.Contains(lowerURL, "bugcrowd.com") {
		return "Bugcrowd"
	}
	if strings.Contains(lowerURL, "intigriti.com") {
		return "Intigriti"
	}
	if strings.Contains(lowerURL, "yeswehack.com") {
		return "YesWeHack"
	}
	// Add more platform checks if needed
	return "Other" // Default platform
}

// aggregateAndFilterScopeData processes all scope entries into program summaries for the compact view.
// Search query applies to ProgramURL, Platform, AND individual scope elements.
func aggregateAndFilterScopeData(allEntries []ScopeEntry, searchQuery string) []ProgramSummaryEntry {
	programMap := make(map[string]*ProgramSummaryEntry) // Key: ProgramURL
	searchLower := strings.ToLower(strings.TrimSpace(searchQuery))

	// First, aggregate counts, identify platforms, and collect all elements per program
	for _, entry := range allEntries {
		progURL := strings.TrimSpace(entry.ProgramURL)
		if progURL == "" {
			continue
		}

		summary, exists := programMap[progURL]
		if !exists {
			summary = &ProgramSummaryEntry{
				ProgramURL: progURL,
				Platform:   getPlatformFromURL(progURL),
				Elements:   make([]string, 0), // Initialize slice for elements
			}
			programMap[progURL] = summary
		}

		summary.Elements = append(summary.Elements, entry.Element) // Add the raw element

		if strings.Contains(entry.Element, "(OOS)") {
			summary.OutOfScopeCount++
		} else {
			summary.InScopeCount++
		}
	}

	// Now, filter the aggregated summaries.
	summaries := make([]ProgramSummaryEntry, 0, len(programMap))

	if searchLower == "" { // No search query, include all aggregated programs
		for _, summary := range programMap {
			summaries = append(summaries, *summary)
		}
	} else { // Search query is present
		for _, summary := range programMap {
			// Check 1: ProgramURL matches
			if strings.Contains(strings.ToLower(summary.ProgramURL), searchLower) {
				summaries = append(summaries, *summary)
				continue // Program matched, move to next program summary
			}

			// Check 2: Platform matches
			if strings.Contains(strings.ToLower(summary.Platform), searchLower) {
				summaries = append(summaries, *summary)
				continue // Program matched
			}

			// Check 3: Any of the program's scope elements match
			foundElementMatch := false
			for _, element := range summary.Elements {
				// Prepare element for search: lowercase and remove "(OOS)" suffix for matching
				elementSearchable := strings.ToLower(element)
				if strings.HasSuffix(elementSearchable, " (oos)") {
					elementSearchable = strings.TrimSuffix(elementSearchable, " (oos)")
				}

				if strings.Contains(elementSearchable, searchLower) {
					foundElementMatch = true
					break // Found a matching element for this program
				}
			}

			if foundElementMatch {
				summaries = append(summaries, *summary)
			}
		}
	}
	return summaries
}

// sortProgramSummaries sorts a slice of ProgramSummaryEntry.
func sortProgramSummaries(summaries []ProgramSummaryEntry, sortBy, sortOrder string) {
	// Default sort if sortBy is empty but order is present
	if sortBy == "" && (sortOrder == SortOrderAsc || sortOrder == SortOrderDesc) {
		sortBy = SortByProgramURL // Default to ProgramURL for compact view
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		var less bool
		progI := summaries[i]
		progJ := summaries[j]

		switch sortBy {
		case SortByProgramURL:
			less = strings.ToLower(progI.ProgramURL) < strings.ToLower(progJ.ProgramURL)
		case SortByInScopeCount:
			if progI.InScopeCount == progJ.InScopeCount { // Secondary sort by URL for stable order
				less = strings.ToLower(progI.ProgramURL) < strings.ToLower(progJ.ProgramURL)
			} else {
				less = progI.InScopeCount < progJ.InScopeCount
			}
		case SortByOutOfScopeCount:
			if progI.OutOfScopeCount == progJ.OutOfScopeCount { // Secondary sort by URL
				less = strings.ToLower(progI.ProgramURL) < strings.ToLower(progJ.ProgramURL)
			} else {
				less = progI.OutOfScopeCount < progJ.OutOfScopeCount
			}
		case SortByPlatformName:
			if strings.ToLower(progI.Platform) == strings.ToLower(progJ.Platform) { // Secondary sort by URL
				less = strings.ToLower(progI.ProgramURL) < strings.ToLower(progJ.ProgramURL)
			} else {
				less = strings.ToLower(progI.Platform) < strings.ToLower(progJ.Platform)
			}
		default:
			// Default to sorting by ProgramURL if sortBy is unrecognized but sortOrder is present
			less = strings.ToLower(progI.ProgramURL) < strings.ToLower(progJ.ProgramURL)
		}

		if sortOrder == SortOrderDesc {
			return !less
		}
		return less
	})
}

// ScopeContent component for the /scope page
func ScopeContent(
	paginatedDetailedEntries []ScopeEntry,
	paginatedProgramSummaries []ProgramSummaryEntry,
	allScopeEntries []ScopeEntry, // Added to access full details for compact view expansion
	loadErr error,
	currentPage int, totalPages int, // Pagination info
	totalResults int, // For "Showing X to Y of Z results"
	search string,
	currentSortBy, currentSortOrder string,
	currentPerPage int,
	hideOOS bool, // For detailed view's OOS toggle state
	showDetailedView bool, // To decide which view to render
	isGoogleBot bool, // To know if we should expand all rows
) g.Node {
	pageContent := []g.Node{
		H1(Class("text-3xl md:text-4xl font-bold text-gray-800 mb-6"), g.Text("Scope Data")),
	}

	if loadErr != nil {
		pageContent = append(pageContent,
			Div(Class("bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4"),
				Strong(g.Text("Error: ")),
				g.Text("Could not load scope data. "+loadErr.Error()),
			),
		)
	} else {
		// Search bar - preserve detailedView state
		searchBar := Div(Class("mb-6"),
			Form(Method("GET"), Action("/scope"), Class("flex flex-col sm:flex-row gap-2 items-stretch sm:items-center"),
				Input(
					Type("text"),
					Name("search"),
					Value(search),
					Placeholder("Search scope..."), // Placeholder can be generic
					Class("flex-1 px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent shadow-sm"),
				),
				Input(Type("hidden"), Name("perPage"), Value(strconv.Itoa(currentPerPage))),
				Input(Type("hidden"), Name("sortBy"), Value(currentSortBy)),
				Input(Type("hidden"), Name("sortOrder"), Value(currentSortOrder)),
				Input(Type("hidden"), Name("detailedView"), Value(strconv.FormatBool(showDetailedView))), // Preserve view mode
				g.If(showDetailedView, // Only include hideOOS if in detailed view
					Input(Type("hidden"), Name("hideOOS"), Value(strconv.FormatBool(hideOOS))),
				),
				Button(
					Type("submit"),
					Class("px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors shadow-sm"),
					g.Text("Search"),
				),
				g.If(search != "",
					A(
						Href(fmt.Sprintf("/scope?perPage=%d&sortBy=%s&sortOrder=%s&detailedView=%t%s",
							currentPerPage, currentSortBy, currentSortOrder, showDetailedView,
							func() string { // Conditional hideOOS query parameter
								if showDetailedView {
									return fmt.Sprintf("&hideOOS=%t", hideOOS)
								}
								return ""
							}(),
						)),
						Class("px-4 py-2 bg-gray-500 text-white rounded-lg hover:bg-gray-600 transition-colors text-center shadow-sm"),
						g.Text("Clear"),
					),
				),
			),
		)
		pageContent = append(pageContent, searchBar)

		// Results Count Text
		var resultsCountText string
		var itemType string
		if showDetailedView {
			itemType = "assets"
		} else {
			itemType = "programs"
		}

		if totalResults > 0 {
			resultsCountText = fmt.Sprintf("Showing %d to %d of %d %s.",
				min((currentPage-1)*currentPerPage+1, totalResults),
				min(currentPage*currentPerPage, totalResults),
				totalResults,
				itemType,
			)
		} else {
			if search != "" {
				resultsCountText = fmt.Sprintf("No %s found for '%s'.", itemType, search)
			} else {
				resultsCountText = fmt.Sprintf("No %s to display.", itemType)
			}
		}

		// View Toggle Button
		hideOOSQueryParam := ""
		if !showDetailedView { // If current is compact, next is detailed, preserve hideOOS state for detailed view
			hideOOSQueryParam = fmt.Sprintf("&hideOOS=%t", hideOOS)
		} else {
			// If current is detailed, next is compact, hideOOS is not relevant for compact, so it's empty.
			// Also, when switching to compact, we might want to reset sort order or use compact view defaults.
			// For now, let's keep it simple and just toggle detailedView.
			// The handler will pick default sorts if sortBy/sortOrder are not applicable to the new view.
		}

		viewToggleURL := fmt.Sprintf("/scope?page=1&search=%s&perPage=%d&detailedView=%t%s",
			url.QueryEscape(search),
			currentPerPage,
			!showDetailedView, // Toggle the view state
			hideOOSQueryParam,
		)

		viewToggleText := "Show Detailed View"
		if showDetailedView {
			viewToggleText = "Show Compact View"
		}
		viewToggleButton := A(Href(viewToggleURL), Class("w-full sm:w-auto text-center px-3 py-2 text-sm font-medium border rounded-md shadow-sm bg-indigo-100 text-indigo-700 border-indigo-300 hover:bg-indigo-200 transition-colors"), g.Text(viewToggleText))

		// Hide OOS Toggle (only for detailed view)
		var hideOOSToggleElement g.Node
		if showDetailedView {
			hideOOSToggleURL := fmt.Sprintf("/scope?page=1&search=%s&sortBy=%s&sortOrder=%s&perPage=%d&hideOOS=%t&detailedView=true",
				url.QueryEscape(search),
				currentSortBy,
				currentSortOrder,
				currentPerPage,
				!hideOOS, // Toggle the current state
			)
			hideOOSToggleText := "Hide OOS"
			hideOOSToggleElementClass := "w-full sm:w-auto text-center px-3 py-2 text-sm font-medium border rounded-md shadow-sm cursor-pointer "
			if hideOOS {
				hideOOSToggleText = "Show OOS"
				hideOOSToggleElementClass += "bg-sky-100 text-sky-700 border-sky-300 hover:bg-sky-200"
			} else {
				hideOOSToggleElementClass += "bg-white text-gray-700 border-gray-300 hover:bg-gray-50"
			}
			hideOOSToggleElement = A(Href(hideOOSToggleURL), Class(hideOOSToggleElementClass), g.Text(hideOOSToggleText))
		}

		// Right side controls: View Toggle, [Hide OOS], PerPage selector
		rightControlsItems := []g.Node{
			viewToggleButton,
		}
		if showDetailedView && hideOOSToggleElement != nil {
			rightControlsItems = append(rightControlsItems, hideOOSToggleElement)
		}
		rightControlsItems = append(rightControlsItems, perPageSelectorForm(search, currentSortBy, currentSortOrder, currentPerPage, hideOOS, showDetailedView))

		rightControls := Div(Class("flex flex-col items-stretch gap-2 sm:flex-row sm:items-center sm:gap-3 w-full sm:w-auto"),
			g.Group(rightControlsItems),
		)

		controlsRow := Div(Class("flex flex-col sm:flex-row justify-between items-center mb-4 gap-4"),
			Div(Class("text-sm text-gray-700"), g.Text(resultsCountText)),
			rightControls,
		)
		pageContent = append(pageContent, controlsRow)

		// Pagination (Top)
		if totalPages > 1 {
			paginationTop := createPagination(currentPage, totalPages, search, currentSortBy, currentSortOrder, currentPerPage, hideOOS, showDetailedView)
			pageContent = append(pageContent, Div(Class("mb-6 flex justify-center"), paginationTop))
		}

		// Helper to build sort URL for table headers
		buildHeaderSortURL := func(targetSortBy string) string {
			order := SortOrderAsc
			if currentSortBy == targetSortBy && currentSortOrder != "" { // Only toggle if it's the same column and already sorted
				if currentSortOrder == SortOrderAsc {
					order = SortOrderDesc
				} else { // Was SortOrderDesc
					order = "" // Clear sort by making order empty
				}
			}
			// If currentSortBy is different, or currentSortOrder is empty, start with 'asc' for targetSortBy

			u := fmt.Sprintf("/scope?page=1&sortBy=%s&sortOrder=%s&perPage=%d&detailedView=%t",
				targetSortBy, order, currentPerPage, showDetailedView)
			if search != "" {
				u += "&search=" + url.QueryEscape(search)
			}
			if showDetailedView { // Only include hideOOS if in detailed view
				u += "&hideOOS=" + strconv.FormatBool(hideOOS)
			}
			return u
		}

		getSortIndicator := func(targetSortBy string) string {
			if currentSortBy == targetSortBy {
				if currentSortOrder == SortOrderAsc {
					return " ▲"
				} else if currentSortOrder == SortOrderDesc {
					return " ▼"
				}
			}
			return ""
		}

		var tableHeaders []g.Node
		var tableRows []g.Node

		if showDetailedView {
			// Table Headers for Detailed View
			tableHeaders = []g.Node{
				Th(Class("px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-1/2"), // Element
					A(Href(buildHeaderSortURL(SortByElement)), Class("hover:text-gray-700 transition-colors"),
						g.Text("Element"+getSortIndicator(SortByElement)),
					),
				),
				Th(Class("px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-1/12"), // Category
					A(Href(buildHeaderSortURL(SortByCategory)), Class("hover:text-gray-700 transition-colors"),
						g.Text("Category"+getSortIndicator(SortByCategory)),
					),
				),
				Th(Class("px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-5/12"), // Program URL
					A(Href(buildHeaderSortURL(SortByProgramURL)), Class("hover:text-gray-700 transition-colors"),
						g.Text("Program URL"+getSortIndicator(SortByProgramURL)),
					),
				),
			}
			// Table Rows for Detailed View
			if len(paginatedDetailedEntries) == 0 {
				noResultsMsg := "No scope entries to display."
				if search != "" {
					noResultsMsg = fmt.Sprintf("No results found for '%s'.", search)
				} else if hideOOS {
					noResultsMsg = "No in-scope entries to display."
				}
				tableRows = append(tableRows,
					Tr(Td(ColSpan("3"), Class("text-center py-10 text-gray-500"), g.Text(noResultsMsg))),
				)
			} else {
				for _, entry := range paginatedDetailedEntries {
					tableRows = append(tableRows,
						Tr(Class("border-b border-gray-200 hover:bg-gray-50 transition-colors"),
							Td(Class("px-4 py-4 text-sm text-gray-900 w-1/2 break-all"), g.Attr("title", entry.Element), g.Text(entry.Element)),
							Td(Class("px-4 py-4 text-sm text-gray-900 w-1/12 break-words"), g.Attr("title", entry.Category), g.Text(strings.ToUpper(entry.Category))),
							Td(Class("px-4 py-4 text-sm text-gray-900 w-5/12 break-words"), // Adjusted width
								A(Href(entry.ProgramURL), Target("_blank"), Rel("noopener noreferrer"),
									Class("text-blue-600 hover:text-blue-800 hover:underline"),
									g.Attr("title", entry.ProgramURL),
									g.Text(entry.ProgramURL),
								),
							),
						),
					)
				}
			}
		} else { // Compact View
			// Table Headers for Compact View
			tableHeaders = []g.Node{
				Th(Class("px-2 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-10")), // Expander column
				Th(Class("px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-2/5"), // URL
					A(Href(buildHeaderSortURL(SortByProgramURL)), Class("hover:text-gray-700 transition-colors"),
						g.Text("URL"+getSortIndicator(SortByProgramURL)),
					),
				),
				Th(Class("px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-1/5"), // In Scope
					A(Href(buildHeaderSortURL(SortByInScopeCount)), Class("hover:text-gray-700 transition-colors"),
						g.Text("In Scope"+getSortIndicator(SortByInScopeCount)),
					),
				),
				Th(Class("px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-1/5"), // Out of Scope
					A(Href(buildHeaderSortURL(SortByOutOfScopeCount)), Class("hover:text-gray-700 transition-colors"),
						g.Text("Out of Scope"+getSortIndicator(SortByOutOfScopeCount)),
					),
				),
				Th(Class("px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-1/5"), // Platform
					A(Href(buildHeaderSortURL(SortByPlatformName)), Class("hover:text-gray-700 transition-colors"),
						g.Text("Platform"+getSortIndicator(SortByPlatformName)),
					),
				),
			}
			// Table Rows for Compact View
			if len(paginatedProgramSummaries) == 0 {
				noResultsMsg := "No programs to display."
				if search != "" {
					noResultsMsg = fmt.Sprintf("No programs found for '%s'.", search)
				}
				tableRows = append(tableRows,
					Tr(Td(ColSpan("5"), Class("text-center py-10 text-gray-500"), g.Text(noResultsMsg))), // ColSpan is now 5
				)
			} else {
				for i, summary := range paginatedProgramSummaries {
					detailsID := fmt.Sprintf("details-%d-%d", currentPage, i) // Unique ID for the details row
					iconID := fmt.Sprintf("icon-%d-%d", currentPage, i)       // Unique ID for the icon

					expanderIcon := "+"
					detailsRowClass := "hidden"
					if isGoogleBot {
						expanderIcon = "-"
						detailsRowClass = "" // Not hidden
					}

					compactRow := Tr(
						Class("border-b border-gray-200 hover:bg-gray-50 transition-colors cursor-pointer"), // Added cursor-pointer
						g.Attr("onclick", fmt.Sprintf("toggleScopeDetails('%s', '%s')", detailsID, iconID)), // Moved onclick here
						Td(Class("px-2 py-3 text-center"), // Removed cursor-pointer and onclick from here
							Span(ID(iconID), Class("expand-icon font-mono text-lg text-blue-600 hover:text-blue-800"), g.Text(expanderIcon)),
						),
						Td(Class("px-4 py-3 text-sm text-gray-900 w-2/5 break-words"),
							A(Href(summary.ProgramURL), Target("_blank"), Rel("noopener noreferrer"),
								Class("text-blue-600 hover:text-blue-800 hover:underline"),
								g.Attr("title", summary.ProgramURL),
								g.Attr("onclick", "event.stopPropagation();"), // Added to prevent row click when link is clicked
								g.Text(summary.ProgramURL),
							),
						),
						Td(Class("px-4 py-3 text-sm text-gray-900 w-1/5 text-center"), g.Text(strconv.Itoa(summary.InScopeCount))),
						Td(Class("px-4 py-3 text-sm text-gray-900 w-1/5 text-center"), g.Text(strconv.Itoa(summary.OutOfScopeCount))),
						Td(Class("px-4 py-3 text-sm text-gray-900 w-1/5"), g.Text(summary.Platform)),
					)
					tableRows = append(tableRows, compactRow)

					// Filter allScopeEntries for this program's details
					var programDetailedEntriesInScope []ScopeEntry
					var programDetailedEntriesOutOfScope []ScopeEntry
					for _, entry := range allScopeEntries {
						if entry.ProgramURL == summary.ProgramURL {
							// Element field already contains "(OOS)" if it's out of scope, as per current aggregation logic
							if strings.Contains(strings.ToLower(entry.Element), "(oos)") {
								programDetailedEntriesOutOfScope = append(programDetailedEntriesOutOfScope, entry)
							} else {
								programDetailedEntriesInScope = append(programDetailedEntriesInScope, entry)
							}
						}
					}

					detailsContentNodes := []g.Node{}
					if len(programDetailedEntriesInScope) > 0 {
						detailsContentNodes = append(detailsContentNodes, H4(Class("font-semibold text-gray-700 mb-1 text-base"), g.Text("In Scope Assets:")))
						inScopeListItems := []g.Node{}
						for _, entry := range programDetailedEntriesInScope {
							assetText := entry.Element
							if entry.Category != "" {
								assetText = fmt.Sprintf("%s: %s", entry.Category, entry.Element)
							}
							inScopeListItems = append(inScopeListItems, Li(Class("ml-4 text-sm text-gray-600"), g.Text(assetText)))
						}
						detailsContentNodes = append(detailsContentNodes, Ul(Class("list-disc list-inside mb-3"), g.Group(inScopeListItems)))
					}

					if len(programDetailedEntriesOutOfScope) > 0 {
						detailsContentNodes = append(detailsContentNodes, H4(Class("font-semibold text-gray-700 mb-1 text-base"), g.Text("Out of Scope Assets:")))
						outOfScopeListItems := []g.Node{}
						for _, entry := range programDetailedEntriesOutOfScope {
							assetText := entry.Element // Element already contains (OOS)
							if entry.Category != "" {
								assetText = fmt.Sprintf("%s: %s", entry.Category, entry.Element)
							}
							outOfScopeListItems = append(outOfScopeListItems, Li(Class("ml-4 text-sm text-gray-600"), g.Text(assetText)))
						}
						detailsContentNodes = append(detailsContentNodes, Ul(Class("list-disc list-inside"), g.Group(outOfScopeListItems)))
					}

					if len(programDetailedEntriesInScope) == 0 && len(programDetailedEntriesOutOfScope) == 0 {
						detailsContentNodes = append(detailsContentNodes, P(Class("text-sm text-gray-500 py-2"), g.Text("No detailed asset information available for this program.")))
					}

					detailsRow := Tr(
						ID(detailsID),
						Class(detailsRowClass), // Initially hidden, unless googlebot
						Td(
							ColSpan("5"), // Span all 5 columns
							Class("p-0"), // No padding on td itself, inner div handles it
							Div(Class("p-4 bg-gray-50 border-t border-b border-gray-200"), // Outer container for details section styling
								Div(Class("p-3 rounded bg-white shadow-inner"), // MODIFIED: Removed max-h-60 and overflow-y-auto
									g.Group(detailsContentNodes),
								),
							),
						),
					)
					tableRows = append(tableRows, detailsRow)
				}
			}
		}

		table := Div(Class("overflow-x-auto shadow-md sm:rounded-lg mt-0"),
			Table(Class("min-w-full divide-y divide-gray-200 table-fixed"),
				THead(Class("bg-gray-50"),
					Tr(tableHeaders...),
				),
				TBody(Class("bg-white divide-y divide-gray-200"),
					g.Group(tableRows),
				),
			),
		)
		pageContent = append(pageContent, table)

		// Pagination (Bottom)
		if totalPages > 1 {
			paginationBottom := createPagination(currentPage, totalPages, search, currentSortBy, currentSortOrder, currentPerPage, hideOOS, showDetailedView)
			pageContent = append(pageContent, Div(Class("mt-6 flex justify-center"), paginationBottom))
		}
	}

	return Main(Class("container mx-auto mt-8 mb-16 p-4"),
		Section(Class("md:bg-white md:rounded-lg md:shadow-xl md:p-8 lg:p-12"),
			g.Group(pageContent),
		),
	)
}

// createPagination creates pagination controls
func createPagination(currentPage, totalPages int, search string, sortBy string, sortOrder string, perPage int, hideOOS bool, showDetailedView bool) g.Node {
	var paginationItems []g.Node

	// Helper function to create pagination link
	createPageLink := func(page int, text string, disabled bool, active bool) g.Node {
		// Preserve all relevant query parameters
		href := fmt.Sprintf("/scope?page=%d&perPage=%d&detailedView=%t", page, perPage, showDetailedView)
		if search != "" {
			href += "&search=" + url.QueryEscape(search)
		}
		if sortBy != "" { // sortBy and sortOrder are passed as current, should be fine
			href += "&sortBy=" + sortBy
		}
		if sortOrder != "" {
			href += "&sortOrder=" + sortOrder
		}
		if showDetailedView { // Only include hideOOS if in detailed view
			href += "&hideOOS=" + strconv.FormatBool(hideOOS)
		}

		classes := "px-3 py-2 text-sm font-medium border rounded-md shadow-sm"
		if disabled {
			classes += " bg-gray-100 text-gray-400 cursor-not-allowed"
			return Span(Class(classes), g.Text(text))
		} else if active {
			classes += " bg-blue-600 text-white border-blue-600"
		} else {
			classes += " bg-white text-gray-700 border-gray-300 hover:bg-gray-50"
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
				Span(Class("px-3 py-2 text-sm text-gray-500"), g.Text("...")),
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
				Span(Class("px-3 py-2 text-sm text-gray-500"), g.Text("...")),
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
	PageLayout(
		"bbscope.com - A bug bounty scope aggregator",
		"Find and track bug bounty program scope data from HackerOne, Bugcrowd and other platforms. Search thousands of security targets and monitor scope changes.",
		Navbar(),
		MainContent(),
		FooterEl(),
		"",    // No canonical URL for the home page
		false, // Not a noindex page
	).Render(w)
}

// HTTP handler for the /scope page
func scopeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/scope" {
		http.NotFound(w, r)
		return
	}

	// Get query parameters
	query := r.URL.Query()
	pageStr := query.Get("page")
	search := strings.TrimSpace(query.Get("search"))
	sortByParam := strings.ToLower(strings.TrimSpace(query.Get("sortBy")))
	sortOrderParam := strings.ToLower(strings.TrimSpace(query.Get("sortOrder")))
	perPageStr := query.Get("perPage")
	hideOOSStr := query.Get("hideOOS")
	showDetailedViewStr := query.Get("detailedView")

	showDetailedView := showDetailedViewStr == "true" // Default to false (compact view)

	// Check if the user agent is Googlebot
	userAgent := r.Header.Get("User-Agent")
	isGoogleBot := strings.Contains(strings.ToLower(userAgent), "googlebot")

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	currentPerPage := 50
	allowedPerPages := []int{25, 50, 100, 250, 500}
	if perPageStr != "" {
		if p, err := strconv.Atoi(perPageStr); err == nil {
			isValidPerPage := false
			for _, allowed := range allowedPerPages {
				if p == allowed {
					currentPerPage = p
					isValidPerPage = true
					break
				}
			}
			if !isValidPerPage {
				currentPerPage = 50 // Default if invalid
			}
		}
	}

	hideOOS := false      // Default for hideOOS
	if showDetailedView { // Only parse hideOOS if in detailed view mode
		hideOOS = hideOOSStr == "true"
	}

	activeSortBy := sortByParam
	activeSortOrder := sortOrderParam

	// Validate and default sortBy and sortOrder based on the current view
	if activeSortOrder == SortOrderAsc || activeSortOrder == SortOrderDesc {
		if showDetailedView {
			isValidSortBy := false
			for _, validCol := range []string{SortByElement, SortByCategory, SortByProgramURL} {
				if activeSortBy == validCol {
					isValidSortBy = true
					break
				}
			}
			if !isValidSortBy {
				activeSortBy = SortByElement // Default for detailed view
			}
		} else { // Compact view
			isValidSortBy := false
			for _, validCol := range []string{SortByProgramURL, SortByInScopeCount, SortByOutOfScopeCount, SortByPlatformName} {
				if activeSortBy == validCol {
					isValidSortBy = true
					break
				}
			}
			if !isValidSortBy {
				activeSortBy = SortByProgramURL // Default for compact view
			}
		}
	} else if activeSortOrder != "" {
		activeSortOrder = ""
	}

	allEntries, err := loadScopeFromDB()

	var paginatedDetailedEntries []ScopeEntry
	var paginatedProgramSummaries []ProgramSummaryEntry
	var totalResults, totalPages int

	if showDetailedView {
		processedEntries := filterEntries(allEntries, search, hideOOS)
		if len(processedEntries) > 0 {
			sortEntries(processedEntries, activeSortBy, activeSortOrder)
		}
		totalResults = len(processedEntries)
		if totalResults > 0 {
			totalPages = (totalResults + currentPerPage - 1) / currentPerPage
			if totalPages == 0 {
				totalPages = 1
			}
		}
		if page > totalPages && totalPages > 0 {
			page = totalPages
		}
		if page < 1 && totalPages > 0 {
			page = 1
		} else if page < 1 {
			page = 1
		} // Ensure page is at least 1
		paginatedDetailedEntries = paginateEntries(processedEntries, page, currentPerPage)
	} else { // Compact View
		programSummaries := aggregateAndFilterScopeData(allEntries, search)
		if len(programSummaries) > 0 {
			sortProgramSummaries(programSummaries, activeSortBy, activeSortOrder)
		}
		totalResults = len(programSummaries)
		if totalResults > 0 {
			totalPages = (totalResults + currentPerPage - 1) / currentPerPage
			if totalPages == 0 {
				totalPages = 1
			}
		}
		if page > totalPages && totalPages > 0 {
			page = totalPages
		}
		if page < 1 && totalPages > 0 {
			page = 1
		} else if page < 1 {
			page = 1
		} // Ensure page is at least 1
		paginatedProgramSummaries = paginateProgramSummaries(programSummaries, page, currentPerPage)
	}

	canonicalURL := fmt.Sprintf("/scope?page=%d", page) // Canonical URL always points to the compact view without detailedView param

	// Determine page title for /scope based on current page
	pageTitle := fmt.Sprintf("Scope data - bbscope.com (Page %d)", page)

	// Determine page description for /scope based on current page
	pageDescription := "Browse and download bug bounty scope data from all bug bounty platforms. Find in-scope websites from HackerOne, Bugcrowd, Intigriti and YesWeHack."
	if page > 1 {
		pageDescription = fmt.Sprintf("%s (Page %d)", pageDescription, page)
	}

	PageLayout(
		pageTitle,
		pageDescription,
		Navbar(),
		ScopeContent(
			paginatedDetailedEntries,
			paginatedProgramSummaries,
			allEntries, // Pass allEntries here
			err,
			page, totalPages, totalResults,
			search, activeSortBy, activeSortOrder, currentPerPage,
			hideOOS, showDetailedView,
			isGoogleBot,
		),
		FooterEl(),
		canonicalURL,
		showDetailedView, // Pass showDetailedView to PageLayout for noindex
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

// perPageSelectorForm generates the form for selecting items per page
func perPageSelectorForm(currentSearch, currentSortBy, currentSortOrder string, currentPerPage int, currentHideOOS bool, currentDetailedView bool) g.Node {
	options := []g.Node{}
	allowedPerPages := []int{25, 50, 100, 250, 500}
	for _, num := range allowedPerPages {
		opt := Option(Value(strconv.Itoa(num)), g.Text(fmt.Sprintf("%d items", num)))
		if num == currentPerPage {
			opt = Option(Value(strconv.Itoa(num)), g.Text(fmt.Sprintf("%d items", num)), Selected())
		}
		options = append(options, opt)
	}

	return Form(Method("GET"), Action("/scope"), Class("w-full sm:w-auto flex items-center justify-center sm:justify-start gap-1 sm:gap-2 text-sm"), // MODIFIED CLASS
		Label(For("perPageSelect"), Class("text-gray-700 whitespace-nowrap"), g.Text("Items per page:")),
		Select(
			ID("perPageSelect"),
			Name("perPage"),
			g.Attr("onchange", "this.form.submit()"),
			Class("px-2 py-1 border border-gray-300 rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500 text-sm"),
			g.Group(options),
		),
		// Hidden fields to preserve other parameters
		Input(Type("hidden"), Name("search"), Value(currentSearch)),
		Input(Type("hidden"), Name("sortBy"), Value(currentSortBy)),
		Input(Type("hidden"), Name("sortOrder"), Value(currentSortOrder)),
		Input(Type("hidden"), Name("detailedView"), Value(strconv.FormatBool(currentDetailedView))), // Add detailedView
		g.If(currentDetailedView, // Only include hideOOS if in detailed view
			Input(Type("hidden"), Name("hideOOS"), Value(strconv.FormatBool(currentHideOOS))),
		),
		Input(Type("hidden"), Name("page"), Value("1")), // Reset to page 1 on perPage change
	)
}

// UpdatesContent renders the main content for the /updates page.
func UpdatesContent(updates []UpdateEntry, currentPage, totalPages int, currentPerPage int, currentSearch string, isGoogleBot bool) g.Node {
	pageContent := []g.Node{
		H1(Class("text-3xl font-bold text-gray-800 mb-4"), g.Text("Scope Updates")),
		P(Class("text-gray-600 mb-6"), g.Text("Recent changes to bug bounty program scopes.")),
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
			Class("flex-1 px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent shadow-sm"), // flex-1 allows input to grow in flex-row
		),
		Button(
			Type("submit"),
			Class("px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors shadow-sm"), // items-stretch on Form will handle width on mobile
			g.Text("Search"),
		),
		// Hidden field to carry over perPage setting if search is performed
		Input(Type("hidden"), Name("perPage"), Value(strconv.Itoa(currentPerPage))),
	)
	pageContent = append(pageContent, controlsHeader)

	var tableRows []g.Node
	if len(updates) == 0 {
		noResultsMsg := "No updates to display."
		if currentSearch != "" {
			noResultsMsg = fmt.Sprintf("No updates found for search '%s'.", currentSearch) // Updated message
		}
		tableRows = append(tableRows,
			Tr(Td(ColSpan("6"), Class("text-center py-10 text-gray-500"), g.Text(noResultsMsg))),
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
					Span(ID(iconID), Class("expand-icon font-mono text-lg text-blue-600 hover:text-blue-800"), g.Text(expanderIcon)),
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
					Strong(Class("text-gray-700"), g.Text(changeTypeDisplay+": "+strings.ToUpper(assetCategory)+": ")),
					Span(Class("text-gray-900 break-all"), g.Text(entry.Asset.Value)),
				)
			}

			rowClasses := "border-b border-gray-200 hover:bg-gray-50 transition-colors"
			var rowOnClick g.Node // Changed from g.AttrBuilder
			if isProgramLevelChange {
				rowClasses += " cursor-pointer"
				rowOnClick = g.Attr("onclick", fmt.Sprintf("toggleScopeDetails('%s', '%s')", detailsID, iconID))
			}

			tableRows = append(tableRows,
				Tr(Class(rowClasses), rowOnClick, ID(rowID),
					firstCell, // Expander or empty cell
					Td(Class("px-4 py-3 text-sm"), assetDisplay),
					Td(Class("px-4 py-3 text-sm text-gray-700"), g.Text(entry.ScopeType)),
					Td(Class("px-4 py-3 text-sm"),
						A(Href(entry.ProgramURL), Target("_blank"), Rel("noopener noreferrer"),
							Class("text-blue-600 hover:text-blue-800 hover:underline"),
							g.Attr("title", entry.ProgramURL),
							g.If(isProgramLevelChange, g.Attr("onclick", "event.stopPropagation();")),
							g.Text(truncateMiddle(entry.ProgramURL, 50)),
						),
					),
					Td(Class("px-4 py-3 text-sm text-gray-700"), g.Text(entry.Platform)),
					Td(Class("px-4 py-3 text-sm text-gray-700"), g.Text(entry.Timestamp.Format("2006-01-02 15:04"))),
				),
			)

			if isProgramLevelChange {
				detailsRowClass := "hidden bg-gray-50"
				if isGoogleBot {
					detailsRowClass = "bg-gray-50" // Not hidden
				}

				var detailsRowContent g.Node
				if len(entry.AssociatedAssets) > 0 {
					detailsContentNodes := []g.Node{}
					detailsContentNodes = append(detailsContentNodes, H4(Class("font-semibold text-gray-700 mb-1 text-base pt-2"), g.Text("Associated Assets:")))
					assetListItems := []g.Node{}
					for _, asset := range entry.AssociatedAssets {
						assetText := asset.Value
						if asset.Category != "" {
							assetText = fmt.Sprintf("%s: %s", strings.ToUpper(asset.Category), asset.Value)
						}
						assetListItems = append(assetListItems, Li(Class("ml-4 text-sm text-gray-600 py-0.5"), g.Text(assetText)))
					}
					detailsContentNodes = append(detailsContentNodes, Ul(Class("list-disc list-inside mb-2"), g.Group(assetListItems)))
					detailsRowContent = g.Group(detailsContentNodes)
				} else {
					// Case where a program was added/removed but somehow no assets were logged with it (edge case)
					detailsRowContent = P(Class("text-sm text-gray-500 py-2"), g.Text("No specific asset details were logged for this program change."))
				}

				detailsRow := Tr(
					ID(detailsID),
					Class(detailsRowClass), // Use computed class
					Td(
						ColSpan("6"), // Span all 6 columns
						Class("p-0"),
						Div(Class("p-3 border-t border-b border-gray-300"),
							Div(Class("p-2 rounded bg-white shadow-inner"),
								detailsRowContent,
							),
						),
					),
				)
				tableRows = append(tableRows, detailsRow)
			}
		}
	}

	table := Div(Class("overflow-x-auto shadow-md sm:rounded-lg mt-0"),
		Table(Class("min-w-full divide-y divide-gray-200 table-fixed"),
			THead(Class("bg-gray-50"),
				Tr(
					Th(Class("px-2 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-10")), // Expander column
					Th(Class("px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-2/6"), g.Text("Asset / Change")),
					Th(Class("px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-1/12"), g.Text("Scope Type")),
					Th(Class("px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-2/6"), g.Text("Program URL")),
					Th(Class("px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-1/12"), g.Text("Platform")),
					Th(Class("px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-1/6"), g.Text("Timestamp")),
				),
			),
			TBody(Class("bg-white divide-y divide-gray-200"),
				g.Group(tableRows),
			),
		),
	)
	pageContent = append(pageContent, table)

	// Pagination
	if totalPages > 1 {
		paginationBottom := createUpdatesPagePagination(currentPage, totalPages, currentPerPage)
		pageContent = append(pageContent, Div(Class("mt-6 flex justify-center"), paginationBottom))
	}

	return Main(Class("container mx-auto mt-8 mb-16 p-4"),
		Section(Class("md:bg-white md:rounded-lg md:shadow-xl md:p-8 lg:p-12"),
			g.Group(pageContent),
		),
	)
}

// createUpdatesPagePagination creates pagination controls for the updates page
func createUpdatesPagePagination(currentPage, totalPages int, perPage int) g.Node {
	var paginationItems []g.Node

	// Helper function to create pagination link
	createPageLink := func(page int, text string, disabled bool, active bool) g.Node {
		href := fmt.Sprintf("/updates?page=%d&perPage=%d", page, perPage)

		classes := "px-3 py-2 text-sm font-medium border rounded-md shadow-sm"
		if disabled {
			classes += " bg-gray-100 text-gray-400 cursor-not-allowed"
			return Span(Class(classes), g.Text(text))
		} else if active {
			classes += " bg-blue-600 text-white border-blue-600"
		} else {
			classes += " bg-white text-gray-700 border-gray-300 hover:bg-gray-50"
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
				Span(Class("px-3 py-2 text-sm text-gray-500"), g.Text("...")),
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
				Span(Class("px-3 py-2 text-sm text-gray-500"), g.Text("...")),
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
	searchQuery := strings.TrimSpace(query.Get("search")) // Changed from "filter"
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
		processedUpdates = filteredUpdates // Replace with filtered results
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
		UpdatesContent(paginatedUpdates, currentPage, totalPages, currentPerPage, searchQuery, isGoogleBot),
		FooterEl(),
		updatesCanonicalURL,
		false, // Not a noindex page
	).Render(w)
}
