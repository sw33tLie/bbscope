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
	OpenAIAPIKey string
	OpenAIModel  string
}

var db *storage.DB
var serverDomain string
var serverStartTime time.Time

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
	Handle           string
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
			Handle:     c.Handle,
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
				Link(Rel("preconnect"), Href("https://fonts.googleapis.com")),
				Link(Rel("preconnect"), Href("https://fonts.gstatic.com"), g.Attr("crossorigin", "")),
				Link(Rel("stylesheet"), Href("https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&family=JetBrains+Mono:wght@700;800&display=swap")),
				Script(Src("https://cdn.tailwindcss.com")),
				Script(Src("https://unpkg.com/htmx.org@2.0.4")),
				Script(g.Raw(`tailwind.config={theme:{extend:{fontFamily:{sans:['Inter','ui-sans-serif','system-ui','sans-serif']},colors:{'bb-dark':'#18181b','bb-blue':'#3b82f6','bb-accent':'#06b6d4','bb-surface':'#27272a'}}}}`)),

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

				// Custom CSS
				StyleEl(g.Raw(`
					* { scroll-behavior: smooth; }
					::selection { background: #0891b2; color: white; }
					a:focus-visible, button:focus-visible, input:focus-visible, select:focus-visible {
						outline: 2px solid #06b6d4;
						outline-offset: 2px;
					}
					::-webkit-scrollbar { width: 8px; height: 8px; }
					::-webkit-scrollbar-track { background: #27272a; border-radius: 10px; }
					::-webkit-scrollbar-thumb { background: #52525b; border-radius: 10px; }
					::-webkit-scrollbar-thumb:hover { background: #71717a; }
					.table-fixed { table-layout: fixed; }
					.prose { --tw-prose-body: #d4d4d8; --tw-prose-headings: #e4e4e7; --tw-prose-links: #22d3ee; --tw-prose-bold: #e4e4e7; --tw-prose-code: #e4e4e7; --tw-prose-pre-bg: #18181b; }
					#hero-particles canvas { border-radius: 1rem; }
					.hero-title { text-shadow: 0 0 40px rgba(6, 182, 212, 0.15); }
					.hero-accent { text-shadow: 0 0 30px rgba(6, 182, 212, 0.4), 0 0 60px rgba(6, 182, 212, 0.15); }
				`)),
			),
			Body(Class("bg-zinc-950 font-sans antialiased leading-normal tracking-tight flex flex-col min-h-screen text-zinc-300"),
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
func Navbar(currentPath string) g.Node {
	navLink := func(href, label string) g.Node {
		isActive := currentPath == href
		base := "block text-center md:inline-block transition-all duration-200 px-3 py-2 rounded-md text-sm font-medium "
		if isActive {
			base += "text-cyan-400 bg-cyan-400/10"
		} else {
			base += "text-zinc-400 hover:text-white hover:bg-zinc-800/50"
		}
		return A(Href(href), Class(base), g.Text(label))
	}

	return Nav(Class("bg-zinc-900/80 backdrop-blur-xl text-white p-4 shadow-lg shadow-black/20 sticky top-0 z-50 border-b border-zinc-700/50"),
		Div(Class("container mx-auto flex justify-between items-center"),
			// Logo/Site Name
			A(Href("/"), Class("text-xl font-bold tracking-tight hover:text-cyan-400 transition-colors duration-200"), g.Text("bbscope.com")),

			// Mobile Menu Button (Hamburger)
			Div(Class("md:hidden"),
				Button(
					ID("mobile-menu-button"),
					Type("button"),
					Class("inline-flex items-center justify-center p-2 rounded-md text-zinc-400 hover:text-white hover:bg-zinc-700 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-white"),
					g.Attr("aria-controls", "mobile-menu"),
					g.Attr("aria-expanded", "false"),
					Span(Class("sr-only"), g.Text("Open main menu")),
					g.Raw(`<svg class="block h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" /></svg>`),
				),
			),

			// Navigation Links
			Div(
				ID("mobile-menu"),
				Class("hidden md:flex md:items-center md:space-x-1 w-full md:w-auto absolute md:relative top-16 left-0 md:top-auto md:left-auto bg-zinc-900/95 backdrop-blur-xl md:bg-transparent shadow-xl md:shadow-none rounded-b-lg md:rounded-none py-3 md:py-0 border-b border-zinc-700/50 md:border-0"),
				navLink("/", "Home"),
				navLink("/scope", "Scope"),
				navLink("/updates", "Updates"),
				navLink("/stats", "Stats"),
				navLink("/docs", "Docs"),
				A(Href("https://github.com/sw33tLie/bbscope"), Target("_blank"), Rel("noopener noreferrer"),
					Class("block text-center md:inline-block text-zinc-400 hover:text-white hover:bg-zinc-800/50 transition-all duration-200 px-3 py-2 rounded-md text-sm font-medium"),
					g.Text("GitHub"),
				),
				navLink("/contact", "Contact"),
			),
		),
	)
}

// MainContent component for the landing page
func statCounter(value int, label string) g.Node {
	return Div(Class("text-center px-6"),
		Div(Class("text-3xl md:text-4xl font-extrabold text-cyan-400 tabular-nums"), StyleAttr("font-family:'JetBrains Mono',monospace"), g.Text(fmt.Sprintf("%d", value))),
		Div(Class("text-xs uppercase tracking-wider text-zinc-500 mt-2 font-medium"), g.Text(label)),
	)
}

func MainContent(totalPrograms, totalAssets, platformCount int) g.Node {
	newHeroSection := Section(Class("rounded-2xl overflow-hidden relative bg-bb-dark"),
		// Particles canvas — positioned absolutely behind content
		Div(ID("hero-particles"), Class("absolute inset-0 z-0")),
		// Hero content overlays on top
		Div(Class("relative z-10 items-center w-full px-5 py-16 mx-auto md:px-12 lg:px-16 max-w-7xl lg:py-28"),
			Div(Class("flex w-full mx-auto text-left"),
				Div(Class("relative inline-flex items-center mx-auto align-middle"),
					Div(Class("text-center"),
						Img(
							Class("block mx-auto mb-6 h-24 w-auto invert"),
							Src("/static/images/bbscope-logo.svg"),
							Alt("bbscope.com logo"),
						),
						H1(Class("max-w-5xl text-3xl font-extrabold leading-tight text-white md:text-5xl lg:text-6xl lg:max-w-7xl hero-title"), StyleAttr("letter-spacing:-0.02em"),
							g.Text("All Bug Bounty "),
							Span(Class("text-cyan-400 hero-accent"), g.Text("Scope")),
							g.Text(" in One Place"),
						),
						P(Class("max-w-2xl mx-auto mt-6 text-base md:text-lg leading-relaxed text-zinc-400"), g.Raw("This website collects public bug bounty targets fetched with <a href='https://github.com/sw33tLie/bbscope' class='text-cyan-400 hover:text-cyan-300 underline'>bbscope cli</a>.<br>We have a few extra tools too!")),
						Div(Class("flex flex-col sm:flex-row justify-center items-center w-full max-w-2xl gap-3 mx-auto mt-6"),
							A(Href("/scope"), Class("px-8 py-3.5 text-base font-semibold text-center text-white transition-all duration-300 bg-cyan-600 lg:px-10 rounded-lg hover:bg-cyan-500 hover:shadow-lg hover:shadow-cyan-500/25 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-offset-zinc-950 focus:ring-cyan-500"), g.Text("View scope")),
							A(Href("/updates"), Class("px-8 py-3.5 text-base font-semibold text-center text-cyan-400 transition-all duration-300 border border-zinc-600 lg:px-10 rounded-lg hover:border-cyan-400 hover:bg-cyan-400/5 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-offset-zinc-950 focus:ring-cyan-500"), g.Text("Latest changes")),
						),
						Div(Class("flex flex-wrap justify-center gap-6 mt-14"),
							statCounter(totalPrograms, "Programs Tracked"),
							statCounter(totalAssets, "Total Assets"),
							statCounter(platformCount, "Platforms"),
						),
					),
				),
			),
		),
		// tsParticles — interactive particle links background
		Script(Src("https://cdn.jsdelivr.net/npm/tsparticles-slim@2/tsparticles.slim.bundle.min.js")),
		Script(g.Raw(`
			(async function(){
				if (typeof tsParticles === 'undefined') return;
				await tsParticles.load("hero-particles", {
					fullScreen: { enable: false },
					background: { color: "transparent" },
					fpsLimit: 60,
					particles: {
						number: { value: 60, density: { enable: true, area: 900 } },
						color: { value: "#06b6d4" },
						opacity: { value: 0.3 },
						size: { value: { min: 1, max: 2 } },
						links: {
							enable: true,
							color: "#06b6d4",
							distance: 150,
							opacity: 0.15,
							width: 1
						},
						move: {
							enable: true,
							speed: 0.8,
							direction: "none",
							outModes: { default: "out" }
						},
						shape: { type: "circle" }
					},
					interactivity: {
						events: {
							onHover: { enable: true, mode: "grab" },
							onClick: { enable: true, mode: "push" }
						},
						modes: {
							grab: { distance: 180, links: { opacity: 0.5 } },
							push: { quantity: 3 }
						}
					},
					detectRetina: true
				});
			})();
		`)),
	)

	return Main(Class("container mx-auto mt-10 mb-20 px-4"),
		newHeroSection,

		// Features Section
		Section(Class("py-8 mb-12"),
			H2(Class("text-2xl font-bold text-center text-zinc-100 mb-10"), g.Text("Use cases")),
			Div(Class("grid md:grid-cols-3 gap-8"),
				featureCard(
					`<svg class="w-5 h-5 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>`,
					"Quick Scope",
					"You can quickly view and download bug bounty scope and use it for your own purposes",
				),
				featureCard(
					`<svg class="w-5 h-5 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"/></svg>`,
					"Track changes",
					"We track all scope changes. Hunt on fresh targets!",
				),
				featureCard(
					`<svg class="w-5 h-5 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"/></svg>`,
					"Stats",
					"Get platform insights and statistics for bug bounty programs.",
				),
			),
		),

		// Call to Action Section
		Section(Class("py-12 bg-zinc-800/20 border border-zinc-700/50 rounded-xl"),
			Div(Class("container mx-auto text-center px-4"),
				H2(Class("text-2xl font-bold text-zinc-100 mb-4"), g.Text("Want to help?")),
				P(Class("text-zinc-400 mb-8 max-w-xl mx-auto"), g.Text("This tool is powered by the bbscope CLI tool. Pull requests are welcome!")),
				A(Href("https://github.com/sw33tLie/bbscope"), Target("_blank"), Rel("noopener noreferrer"), Class("bg-emerald-500 hover:bg-emerald-400 text-white font-semibold py-3 px-8 rounded-lg text-base transition-all duration-300 hover:shadow-lg hover:shadow-emerald-500/25"),
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
		{"h1", "HackerOne"},
		{"bc", "Bugcrowd"},
		{"it", "Intigriti"},
		{"ywh", "YesWeHack"},
	}

	tabs := []g.Node{}
	for _, p := range platforms {
		isActive := currentPlatform == p.Value
		href := basePath + "?page=1"
		if p.Value != "" {
			href += "&platform=" + p.Value
		}
		href += extraParams

		classes := "px-4 py-2 text-sm font-medium rounded-lg transition-all duration-200 "
		if isActive {
			classes += "bg-cyan-500 text-white shadow-md shadow-cyan-500/20"
		} else {
			classes += "bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200 border border-zinc-700/50"
		}

		tabs = append(tabs, A(Href(href), Class(classes), g.Text(p.Label)))
	}

	return Div(Class("flex flex-wrap gap-2 mb-6"), g.Group(tabs))
}

// Helper function for feature cards
func featureCard(icon, title, description string) g.Node {
	return Div(Class("group bg-zinc-800/30 border border-zinc-700/50 shadow-lg rounded-xl p-6 hover:border-cyan-500/40 hover:shadow-cyan-500/5 hover:bg-zinc-800/50 transition-all duration-300"),
		Div(Class("w-10 h-10 rounded-lg bg-cyan-500/10 border border-cyan-500/20 flex items-center justify-center mb-4 group-hover:bg-cyan-500/20 transition-colors duration-300"),
			g.Raw(icon),
		),
		H3(Class("text-lg font-semibold text-zinc-100 mb-2"), g.Text(title)),
		P(Class("text-zinc-400 text-sm leading-relaxed"), g.Text(description)),
	)
}

// FooterEl component (using El suffix to avoid conflict with html.Footer)
func FooterEl() g.Node {
	currentYear := time.Now().Year()
	return Footer(Class("bg-zinc-900/50 text-zinc-500 mt-auto border-t border-zinc-800/50"),
		Div(Class("container mx-auto px-4 py-8"),
			Div(Class("flex flex-col md:flex-row justify-between items-center gap-4"),
				P(Class("text-sm"), g.Raw(fmt.Sprintf("© %d bbscope.com. Made by <a href='https://x.com/sw33tLie' class='text-zinc-400 hover:text-cyan-400 transition-colors duration-200'>sw33tLie</a>", currentYear))),
				Div(Class("flex items-center gap-6"),
					A(Href("https://github.com/sw33tLie/bbscope"), Target("_blank"), Rel("noopener noreferrer"),
						Class("text-zinc-500 hover:text-zinc-300 transition-colors duration-200 text-sm"),
						g.Text("GitHub"),
					),

				),
			),
		),
	)
}

// ScopeContent renders the /scope page shell with client-side controls and an empty container
// that gets populated by scope-table.js via the /api/v1/programs JSON API.
func ScopeContent() g.Node {
	// Build the platform filter dropdown (server-rendered, managed by JS)
	platformDropdown := scopePlatformFilterDropdown()

	// Build the search bar (server-rendered, managed by JS)
	searchBar := scopeSearchBar()

	// Build program type pills (server-rendered, managed by JS)
	typePills := scopeProgramTypeFilter()

	return Main(Class("container mx-auto mt-10 mb-20 px-0 sm:px-4"),
		Section(Class("sm:bg-zinc-900/30 sm:border sm:border-zinc-800/50 sm:rounded-2xl sm:shadow-xl sm:shadow-black/10 px-2 py-4 sm:p-6 md:p-8 lg:p-12"),
			// Header row: title + platform dropdown + search
			Div(Class("flex flex-col md:flex-row md:items-center gap-3 mb-4"),
				Div(Class("flex items-center gap-3"),
					H1(Class("text-2xl md:text-3xl font-bold text-white"), g.Text("Scope Data")),
					platformDropdown,
				),
				Div(Class("flex-1"),
					searchBar,
				),
			),
			typePills,
			// Table container — filled by scope-table.js
			Div(ID("scope-table-container"),
				Div(Class("flex flex-col items-center justify-center py-20 gap-4"),
					g.Raw(`<div class="w-8 h-8 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin"></div>`),
					Span(Class("text-zinc-400 text-sm"), g.Text("Loading scope data...")),
				),
			),
			// Noscript fallback
			g.Raw(`<noscript><div class="text-center py-8 text-zinc-400">JavaScript is required to view the scope table. Browse individual programs from the <a href="/sitemap.xml" class="text-cyan-400 hover:underline">sitemap</a>.</div></noscript>`),
			Script(Src("/static/js/scope-table.js")),
		),
	)
}

// scopeProgramTypeFilter renders pill buttons (All / BBP / VDP) managed by scope-table.js.
func scopeProgramTypeFilter() g.Node {
	types := []struct{ Value, Label string }{
		{"", "All"},
		{"bbp", "BBP"},
		{"vdp", "VDP"},
	}

	var pills []g.Node
	for _, t := range types {
		// Default: first pill active
		classes := "scope-type-pill px-4 py-1.5 text-sm font-medium rounded-lg transition-all duration-200 cursor-pointer "
		if t.Value == "" {
			classes += "bg-cyan-500 text-white shadow-md shadow-cyan-500/20"
		} else {
			classes += "bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200 border border-zinc-700/50"
		}

		pills = append(pills, Span(
			Class(classes),
			g.Attr("data-type", t.Value),
			g.Text(t.Label),
		))
	}

	return Div(Class("flex flex-wrap items-center gap-2 mb-4"),
		Span(Class("text-sm text-zinc-500 mr-1"), g.Text("Program type:")),
		g.Group(pills),
		// Spacer
		Span(Class("mx-2 hidden sm:inline text-zinc-700"), g.Text("|")),
		// AI toggle button — managed by scope-table.js
		Span(
			ID("ai-toggle-btn"),
			Class("ai-toggle-btn px-4 py-1.5 text-sm font-medium rounded-lg transition-all duration-200 bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200 border border-zinc-700/50 cursor-pointer flex items-center gap-1.5"),
			g.Raw(`<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"/></svg>`),
			g.Text("Raw"),
		),
	)
}

// scopePlatformFilterDropdown renders a multiselect dropdown managed by scope-table.js.
func scopePlatformFilterDropdown() g.Node {
	platforms := []struct{ Value, Label string }{
		{"h1", "HackerOne"},
		{"bc", "Bugcrowd"},
		{"it", "Intigriti"},
		{"ywh", "YesWeHack"},
	}

	var checkboxItems []g.Node
	for _, p := range platforms {
		checkboxItems = append(checkboxItems,
			Label(Class("flex items-center gap-2 px-3 py-1.5 hover:bg-zinc-700/50 cursor-pointer text-sm text-zinc-300"),
				Input(
					Type("checkbox"),
					Name("platform"),
					Value(p.Value),
					Class("rounded border-zinc-600 bg-zinc-700 text-cyan-500 focus:ring-cyan-500 focus:ring-offset-0"),
				),
				g.Text(p.Label),
			),
		)
	}

	return Div(Class("relative"), ID("platform-filter"),
		Button(
			Type("button"),
			ID("platform-dropdown-btn"),
			Class("flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-lg bg-zinc-800/50 text-zinc-300 border border-zinc-700 hover:bg-zinc-700 hover:text-zinc-200 hover:border-zinc-600 transition-all duration-200"),
			g.Text("All Platforms"),
			g.Raw(`<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>`),
		),
		Div(
			ID("platform-dropdown-menu"),
			Class("hidden absolute z-30 mt-2 w-56 bg-zinc-800 border border-zinc-700/50 rounded-xl shadow-2xl shadow-black/30 py-1.5"),
			Button(
				Type("button"),
				ID("platform-all-btn"),
				Class("w-full text-left px-3 py-1.5 text-sm text-cyan-400 hover:bg-zinc-700/50 font-medium border-b border-zinc-700 mb-1"),
				g.Text("All Platforms"),
			),
			g.Group(checkboxItems),
			Div(Class("px-3 pt-2 pb-1 border-t border-zinc-700 mt-1"),
				Button(
					Type("button"),
					ID("platform-apply-btn"),
					Class("w-full px-3 py-1.5 text-sm font-medium rounded bg-cyan-600 text-white hover:bg-cyan-500 transition-colors"),
					g.Text("Apply"),
				),
			),
		),
	)
}

// scopeSearchBar renders the search input for the scope page, managed by scope-table.js.
func scopeSearchBar() g.Node {
	return Div(
		Class("flex flex-col sm:flex-row gap-2 items-stretch sm:items-center"),
		Div(Class("relative flex-1"),
			Div(Class("absolute inset-y-0 left-0 flex items-center pl-3 pointer-events-none"),
				g.Raw(`<svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>`),
			),
			Input(
				Type("text"),
				ID("scope-search-input"),
				Placeholder("Search programs and assets..."),
				Class("w-full pl-10 pr-4 py-2.5 border border-zinc-700 rounded-lg focus:ring-2 focus:ring-cyan-500 focus:border-cyan-500 bg-zinc-800/50 text-zinc-200 placeholder-zinc-500 transition-colors duration-200"),
			),
		),
		Button(
			Type("button"),
			ID("scope-search-clear"),
			Class("px-4 py-2.5 bg-zinc-700 text-zinc-300 rounded-lg hover:bg-zinc-600 hover:text-white transition-all duration-200 text-center"),
			g.Text("Clear"),
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
		Navbar("/"),
		MainContent(totalPrograms, totalAssets, platformCount),
		FooterEl(),
		"",
		false,
	).Render(w)
}

// HTTP handler for the /scope page — serves a shell that loads data client-side via the API.
func scopeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/scope" {
		http.NotFound(w, r)
		return
	}

	PageLayout(
		"Scope data - bbscope.com",
		"Browse and download bug bounty scope data from all bug bounty platforms. Find in-scope websites from HackerOne, Bugcrowd, Intigriti and YesWeHack.",
		Navbar("/scope"),
		ScopeContent(),
		FooterEl(),
		"/scope",
		false,
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
	serverStartTime = time.Now()

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
	http.HandleFunc("/debug", debugHandler)

	// Public API
	http.HandleFunc("/api/v1/programs", apiProgramsHandler)
	http.HandleFunc("/api/v1/programs/", apiProgramDetailHandler)

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

// changeTypeBadge renders a colored badge for the change type.
func changeTypeBadge(changeType string) g.Node {
	var label, colors string
	switch changeType {
	case "program_added":
		label = "Program Added"
		colors = "bg-emerald-900/50 text-emerald-300 border border-emerald-800"
	case "program_removed":
		label = "Program Removed"
		colors = "bg-red-900/50 text-red-300 border border-red-800"
	case "asset_added":
		label = "Added"
		colors = "bg-emerald-900/50 text-emerald-300 border border-emerald-800"
	case "asset_removed":
		label = "Removed"
		colors = "bg-red-900/50 text-red-300 border border-red-800"
	default:
		label = changeType
		colors = "bg-zinc-700 text-zinc-300"
	}
	return Span(Class("inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md "+colors), g.Text(label))
}

// scopeBadge renders an in-scope or out-of-scope badge.
func scopeBadge(scopeType string) g.Node {
	if scopeType == "" {
		return Span(Class("text-zinc-500 text-xs"), g.Text("—"))
	}
	colors := "bg-zinc-700 text-zinc-400 border border-zinc-600"
	if scopeType == "In Scope" {
		colors = "bg-emerald-900/30 text-emerald-400 border border-emerald-800"
	} else if scopeType == "Out of Scope" {
		colors = "bg-zinc-800 text-zinc-400 border border-zinc-700"
	}
	return Span(Class("inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md "+colors), g.Text(scopeType))
}

// UpdatesContent renders the main content for the /updates page.
func UpdatesContent(updates []UpdateEntry, currentPage, totalPages int, currentPerPage int, currentSearch string, isGoogleBot bool, currentPlatform string) g.Node {
	pageContent := []g.Node{
		H1(Class("text-2xl md:text-3xl font-bold text-white mb-4"), g.Text("Scope Updates")),
		P(Class("text-zinc-400 mb-6"), g.Text("Recent changes to bug bounty program scopes.")),
		platformFilterTabs("/updates", currentPlatform, fmt.Sprintf("&perPage=%d", currentPerPage)),
	}

	// Search controls
	controlsHeader := Form(Method("GET"), Action("/updates"), Class("flex flex-col sm:flex-row gap-2 items-stretch sm:items-center mb-6"),
		Div(Class("relative flex-1"),
			Div(Class("absolute inset-y-0 left-0 flex items-center pl-3 pointer-events-none"),
				g.Raw(`<svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>`),
			),
			Input(
				Type("text"),
				Name("search"),
				Placeholder("Search updates..."),
				Value(currentSearch),
				Class("w-full pl-10 pr-4 py-2.5 border border-zinc-700 rounded-lg focus:ring-2 focus:ring-cyan-500 focus:border-cyan-500 bg-zinc-800/50 text-zinc-200 placeholder-zinc-500 transition-colors duration-200"),
			),
		),
		Button(
			Type("submit"),
			Class("px-6 py-2.5 bg-cyan-600 text-white font-medium rounded-lg hover:bg-cyan-500 transition-all duration-200 hover:shadow-md hover:shadow-cyan-500/20"),
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
			noResultsMsg = fmt.Sprintf("No updates found for search '%s'.", currentSearch)
		}
		tableRows = append(tableRows,
			Tr(Td(ColSpan("7"), Class("text-center py-16 text-zinc-500"),
				Div(Class("flex flex-col items-center gap-3"),
					g.Raw(`<svg class="w-12 h-12 text-zinc-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>`),
					Span(g.Text(noResultsMsg)),
				),
			)),
		)
	} else {
		for i, entry := range updates {
			detailsID := fmt.Sprintf("update-details-%d-%d", currentPage, i)
			iconID := fmt.Sprintf("update-icon-%d-%d", currentPage, i)

			isProgramLevelChange := entry.Type == "program_added" || entry.Type == "program_removed"

			rowClasses := "border-b border-zinc-800/50 hover:bg-zinc-800/50 transition-colors duration-150"
			if i%2 == 1 {
				rowClasses += " bg-zinc-800/20"
			}

			// Build program link — link to internal program page if handle is available
			var programCell g.Node
			if entry.Handle != "" {
				internalURL := fmt.Sprintf("/program/%s/%s",
					url.PathEscape(strings.ToLower(entry.Platform)),
					url.PathEscape(entry.Handle),
				)
				programCell = Td(Class("px-4 py-3 text-sm"),
					Div(Class("flex items-center gap-2"),
						A(Href(entry.ProgramURL), Target("_blank"), Rel("noopener noreferrer"),
							Class("text-zinc-500 hover:text-cyan-400 transition-colors flex-shrink-0"),
							g.Attr("title", "Open on "+capitalizedPlatform(entry.Platform)),
							g.If(isProgramLevelChange, g.Attr("onclick", "event.stopPropagation();")),
							g.Raw(`<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"/></svg>`),
						),
						A(Href(internalURL),
							Class("text-cyan-400 hover:text-cyan-300 hover:underline transition-colors"),
							g.If(isProgramLevelChange, g.Attr("onclick", "event.stopPropagation();")),
							g.Text(entry.Handle),
						),
					),
				)
			} else {
				programCell = Td(Class("px-4 py-3 text-sm"),
					A(Href(entry.ProgramURL), Target("_blank"), Rel("noopener noreferrer"),
						Class("text-cyan-400 hover:text-cyan-300 hover:underline transition-colors"),
						g.Attr("title", entry.ProgramURL),
						g.If(isProgramLevelChange, g.Attr("onclick", "event.stopPropagation();")),
						g.Text(truncateMiddle(entry.ProgramURL, 40)),
					),
				)
			}

			if isProgramLevelChange {
				expanderIcon := "+"
				if isGoogleBot {
					expanderIcon = "-"
				}
				rowClasses += " cursor-pointer"

				tableRows = append(tableRows,
					Tr(Class(rowClasses),
						g.Attr("onclick", fmt.Sprintf("toggleScopeDetails('%s', '%s')", detailsID, iconID)),
						// Change type
						Td(Class("px-4 py-3 text-sm"),
							Div(Class("flex items-center gap-2"),
								Span(ID(iconID), Class("font-mono text-sm text-cyan-400 hover:text-cyan-300 flex-shrink-0"), g.Text(expanderIcon)),
								changeTypeBadge(entry.Type),
							),
						),
						// Asset (empty for program-level)
						Td(Class("px-4 py-3 text-sm text-zinc-500"), g.Text("—")),
						// Category (empty for program-level)
						Td(Class("px-4 py-3 text-sm text-zinc-500"), g.Text("—")),
						// Scope (empty for program-level)
						Td(Class("px-4 py-3 text-sm text-zinc-500"), g.Text("—")),
						// Program
						programCell,
						// Platform
						Td(Class("px-4 py-3 text-sm"), platformBadge(entry.Platform)),
						// Time
						Td(Class("px-4 py-3 text-sm text-zinc-400 whitespace-nowrap"), g.Text(entry.Timestamp.Format("2006-01-02 15:04"))),
					),
				)

				// Expandable details row
				detailsRowClass := "hidden bg-zinc-800/50"
				if isGoogleBot {
					detailsRowClass = "bg-zinc-800/50"
				}

				var detailsRowContent g.Node
				if len(entry.AssociatedAssets) > 0 {
					var assetItems []g.Node
					for _, asset := range entry.AssociatedAssets {
						cat := strings.ToUpper(asset.Category)
						if cat == "" {
							cat = "OTHER"
						}
						assetItems = append(assetItems,
							Div(Class("flex items-center gap-2 py-1"),
								categoryBadge(cat),
								Span(Class("text-sm text-zinc-300 break-all"), g.Text(asset.Value)),
							),
						)
					}
					detailsRowContent = Div(
						H4(Class("font-semibold text-zinc-300 text-sm mb-2"), g.Text("Associated Assets:")),
						Div(Class("space-y-0.5"), g.Group(assetItems)),
					)
				} else {
					detailsRowContent = P(Class("text-sm text-zinc-400 py-2"), g.Text("No specific asset details were logged for this program change."))
				}

				tableRows = append(tableRows,
					Tr(ID(detailsID), Class(detailsRowClass),
						Td(ColSpan("7"), Class("p-0"),
							Div(Class("p-3 border-t border-b border-zinc-700"),
								Div(Class("p-3 rounded bg-zinc-900/80 shadow-inner"),
									detailsRowContent,
								),
							),
						),
					),
				)
			} else {
				// Asset-level change row
				assetCategory := entry.Asset.Category
				if assetCategory == "" {
					assetCategory = "OTHER"
				}

				tableRows = append(tableRows,
					Tr(Class(rowClasses),
						// Change type
						Td(Class("px-4 py-3 text-sm"),
							changeTypeBadge(entry.Type),
						),
						// Asset
						Td(Class("px-4 py-3 text-sm text-zinc-200 break-all"), g.Text(entry.Asset.Value)),
						// Category
						Td(Class("px-4 py-3 text-sm"),
							categoryBadge(assetCategory),
						),
						// Scope
						Td(Class("px-4 py-3 text-sm"),
							scopeBadge(entry.ScopeType),
						),
						// Program
						programCell,
						// Platform
						Td(Class("px-4 py-3 text-sm"), platformBadge(entry.Platform)),
						// Time
						Td(Class("px-4 py-3 text-sm text-zinc-400 whitespace-nowrap"), g.Text(entry.Timestamp.Format("2006-01-02 15:04"))),
					),
				)
			}
		}
	}

	table := Div(Class("overflow-x-auto rounded-none sm:rounded-xl border-y sm:border border-zinc-700/50 sm:shadow-xl sm:shadow-black/10"),
		Table(Class("min-w-full divide-y divide-zinc-700"),
			THead(Class("bg-zinc-800/80"),
				Tr(
					Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider"), g.Text("Change")),
					Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider"), g.Text("Asset")),
					Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-28"), g.Text("Category")),
					Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-28"), g.Text("Scope")),
					Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider"), g.Text("Program")),
					Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-28"), g.Text("Platform")),
					Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-36"), g.Text("Time")),
				),
			),
			TBody(Class("bg-zinc-900/50 divide-y divide-zinc-800"),
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

	return Main(Class("container mx-auto mt-10 mb-20 px-0 sm:px-4"),
		Section(Class("sm:bg-zinc-900/30 sm:border sm:border-zinc-800/50 sm:rounded-2xl sm:shadow-xl sm:shadow-black/10 px-2 py-4 sm:p-6 md:p-8 lg:p-12"),
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

		classes := "px-3 py-1.5 text-sm font-medium rounded-full transition-all duration-200"
		if disabled {
			classes += " bg-zinc-800/50 text-zinc-600 cursor-not-allowed"
			return Span(Class(classes), g.Text(text))
		} else if active {
			classes += " bg-cyan-600 text-white shadow-md shadow-cyan-500/20"
		} else {
			classes += " bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200"
		}

		return A(Href(href), Class(classes), g.Text(text))
	}

	// Previous button - arrow on mobile, text on desktop
	prevHref := fmt.Sprintf("/updates?page=%d&perPage=%d", currentPage-1, perPage)
	if platform != "" {
		prevHref += "&platform=" + platform
	}
	if currentPage <= 1 {
		paginationItems = append(paginationItems, Span(Class("px-2 py-1.5 text-sm font-medium rounded-full bg-zinc-800/50 text-zinc-600 cursor-not-allowed"),
			g.Raw(`<span class="hidden sm:inline">Previous</span><span class="sm:hidden">&larr;</span>`),
		))
	} else {
		paginationItems = append(paginationItems, A(Href(prevHref), Class("px-2 py-1.5 text-sm font-medium rounded-full bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200 transition-all duration-200"),
			g.Raw(`<span class="hidden sm:inline">Previous</span><span class="sm:hidden">&larr;</span>`),
		))
	}

	// Page numbers - show fewer on mobile
	start := max(1, currentPage-2)
	end := min(totalPages, currentPage+2)

	if start > 1 {
		paginationItems = append(paginationItems, createPageLink(1, "1", false, false))
		if start > 2 {
			paginationItems = append(paginationItems,
				Span(Class("px-1 sm:px-2 py-1.5 text-sm text-zinc-600"), g.Text("...")),
			)
		}
	}

	for i := start; i <= end; i++ {
		hideOnMobile := ""
		if i != currentPage && (i < currentPage-1 || i > currentPage+1) {
			hideOnMobile = " hidden sm:inline-flex"
		}
		pageClasses := "px-3 py-1.5 text-sm font-medium rounded-full transition-all duration-200"
		if i == currentPage {
			pageClasses += " bg-cyan-600 text-white shadow-md shadow-cyan-500/20"
		} else {
			pageClasses += " bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200"
		}
		pageClasses += hideOnMobile

		pageHref := fmt.Sprintf("/updates?page=%d&perPage=%d", i, perPage)
		if platform != "" {
			pageHref += "&platform=" + platform
		}
		paginationItems = append(paginationItems, A(Href(pageHref), Class(pageClasses), g.Text(strconv.Itoa(i))))
	}

	if end < totalPages {
		if end < totalPages-1 {
			paginationItems = append(paginationItems,
				Span(Class("px-1 sm:px-2 py-1.5 text-sm text-zinc-600"), g.Text("...")),
			)
		}
		paginationItems = append(paginationItems,
			createPageLink(totalPages, strconv.Itoa(totalPages), false, false),
		)
	}

	// Next button - arrow on mobile, text on desktop
	nextHref := fmt.Sprintf("/updates?page=%d&perPage=%d", currentPage+1, perPage)
	if platform != "" {
		nextHref += "&platform=" + platform
	}
	if currentPage >= totalPages {
		paginationItems = append(paginationItems, Span(Class("px-2 py-1.5 text-sm font-medium rounded-full bg-zinc-800/50 text-zinc-600 cursor-not-allowed"),
			g.Raw(`<span class="hidden sm:inline">Next</span><span class="sm:hidden">&rarr;</span>`),
		))
	} else {
		paginationItems = append(paginationItems, A(Href(nextHref), Class("px-2 py-1.5 text-sm font-medium rounded-full bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200 transition-all duration-200"),
			g.Raw(`<span class="hidden sm:inline">Next</span><span class="sm:hidden">&rarr;</span>`),
		))
	}

	return Div(Class("mt-6 flex justify-center"),
		Nav(Class("inline-flex items-center gap-1 bg-zinc-800/30 rounded-full px-1 py-1"),
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
		Navbar("/updates"),
		UpdatesContent(paginatedUpdates, currentPage, totalPages, currentPerPage, searchQuery, isGoogleBot, platformFilter),
		FooterEl(),
		updatesCanonicalURL,
		currentPage > 1,
	).Render(w)
}
