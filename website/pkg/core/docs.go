package core

import (
	"net/http"

	g "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

func docsFeatureItem(icon, title, desc string) g.Node {
	return Div(Class("flex gap-4 items-start"),
		Div(Class("w-9 h-9 rounded-lg bg-cyan-500/10 border border-cyan-500/20 flex items-center justify-center flex-shrink-0 mt-0.5"),
			g.Raw(icon),
		),
		Div(
			H3(Class("text-base font-semibold text-zinc-100 mb-1"), g.Text(title)),
			P(Class("text-sm text-zinc-400 leading-relaxed"), g.Text(desc)),
		),
	)
}

// DocsContent component for the /docs page
func DocsContent() g.Node {
	return Main(Class("container mx-auto mt-10 mb-20 px-4 max-w-3xl"),
		// Page header
		Div(Class("mb-10"),
			H1(Class("text-2xl md:text-3xl font-bold text-white mb-3"), g.Text("Documentation")),
			P(Class("text-zinc-400 text-lg"), g.Text("Learn how to use bbscope.com to track bug bounty program scopes.")),
		),

		// Introduction
		Section(Class("bg-zinc-900/30 border border-zinc-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8 mb-6"),
			H2(Class("text-lg font-semibold text-white mb-3"), g.Text("Introduction")),
			P(Class("text-zinc-400 leading-relaxed"), g.Text("This platform aggregates public bug bounty program scopes from HackerOne, Bugcrowd, Intigriti, and YesWeHack. All data is fetched automatically using the bbscope CLI tool.")),
		),

		// Features
		Section(Class("bg-zinc-900/30 border border-zinc-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8 mb-6"),
			H2(Class("text-lg font-semibold text-white mb-5"), g.Text("Features")),
			Div(Class("space-y-5"),
				docsFeatureItem(
					`<svg class="w-4 h-4 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>`,
					"View Scope Data",
					"Browse through aggregated scope information from all major bug bounty platforms.",
				),
				docsFeatureItem(
					`<svg class="w-4 h-4 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z"/></svg>`,
					"Search & Filter",
					"Quickly find specific targets, categories, or programs. Filter by platform and sort by any column.",
				),
				docsFeatureItem(
					`<svg class="w-4 h-4 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"/></svg>`,
					"Track Changes",
					"Monitor scope updates in real time. See when programs add or remove assets.",
				),
				docsFeatureItem(
					`<svg class="w-4 h-4 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"/></svg>`,
					"Quick Links",
					"One-click access to recon tools like crt.sh, Shodan, SecurityTrails, and VirusTotal for every in-scope asset.",
				),
			),
		),

		// How to Use
		Section(Class("bg-zinc-900/30 border border-zinc-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8 mb-6"),
			H2(Class("text-lg font-semibold text-white mb-4"), g.Text("How to Use")),
			Ol(Class("space-y-3 text-zinc-400 list-decimal list-inside"),
				Li(Class("leading-relaxed"),
					g.Text("Navigate to the "),
					A(Href("/scope"), Class("text-cyan-400 hover:text-cyan-300 transition-colors"), g.Text("Scope Data")),
					g.Text(" page to browse all programs."),
				),
				Li(Class("leading-relaxed"), g.Text("Use the search bar to filter by program name or asset value.")),
				Li(Class("leading-relaxed"), g.Text("Click on a program row to see its full scope details and quick links.")),
				Li(Class("leading-relaxed"),
					g.Text("Visit the "),
					A(Href("/updates"), Class("text-cyan-400 hover:text-cyan-300 transition-colors"), g.Text("Updates")),
					g.Text(" page to track recent scope changes."),
				),
			),
		),

		// Contributing
		Section(Class("bg-zinc-900/30 border border-zinc-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8"),
			H2(Class("text-lg font-semibold text-white mb-3"), g.Text("Contributing")),
			P(Class("text-zinc-400 leading-relaxed mb-4"), g.Text("bbscope is an open-source project. Contributions are welcome!")),
			A(Href("https://github.com/sw33tLie/bbscope"), Target("_blank"), Rel("noopener noreferrer"),
				Class("inline-flex items-center gap-2 px-4 py-2 bg-zinc-800/50 border border-zinc-700/50 rounded-lg text-sm text-zinc-300 hover:text-cyan-400 hover:border-zinc-600 transition-all duration-200"),
				g.Raw(`<svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>`),
				g.Text("View on GitHub"),
			),
		),
	)
}

// HTTP handler for the /docs page
func docsHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/docs" {
		http.NotFound(w, r)
		return
	}
	PageLayout(
		"Documentation - bbscope.com",
		"Find out how bbscope.com can help you find new bugs",
		Navbar("/docs"),
		DocsContent(),
		FooterEl(),
		"",
		false,
	).Render(w)
}
