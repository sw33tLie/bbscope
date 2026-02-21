package core

import (
	"fmt"
	"net/http"
	"time"

	g "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

var debugPlatformNames = map[string]string{
	"h1":  "HackerOne",
	"bc":  "Bugcrowd",
	"it":  "Intigriti",
	"ywh": "YesWeHack",
}

func debugContent() g.Node {
	statuses := GetPollerStatuses()
	platformOrder := []string{"h1", "bc", "it", "ywh"}

	var rows []g.Node
	for _, key := range platformOrder {
		displayName := debugPlatformNames[key]
		s, ok := statuses[key]

		var statusBadge, startedAt, duration g.Node

		if !ok {
			statusBadge = Span(Class("inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-zinc-700 text-zinc-400"), g.Text("Pending"))
			startedAt = Span(Class("text-zinc-500"), g.Text("-"))
			duration = Span(Class("text-zinc-500"), g.Text("-"))
		} else if s.Skipped {
			statusBadge = Span(Class("inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-zinc-700 text-zinc-400"), g.Text("Skipped"))
			startedAt = Span(Class("text-zinc-500"), g.Text("-"))
			duration = Span(Class("text-zinc-500"), g.Text("-"))
		} else if s.Success {
			statusBadge = Span(Class("inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-emerald-900/50 text-emerald-400 border border-emerald-800/50"), g.Text("Success"))
			startedAt = Span(g.Text(s.StartedAt.UTC().Format(time.RFC3339)))
			duration = Span(g.Text(s.Duration.Round(time.Second).String()))
		} else {
			statusBadge = Span(Class("inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-900/50 text-red-400 border border-red-800/50"), g.Text("Error"))
			startedAt = Span(g.Text(s.StartedAt.UTC().Format(time.RFC3339)))
			duration = Span(g.Text(s.Duration.Round(time.Second).String()))
		}

		rows = append(rows, Tr(Class("border-b border-zinc-800/50"),
			Td(Class("px-4 py-3 font-medium text-white"), g.Text(displayName)),
			Td(Class("px-4 py-3"), statusBadge),
			Td(Class("px-4 py-3 text-zinc-400 text-sm tabular-nums"), startedAt),
			Td(Class("px-4 py-3 text-zinc-400 text-sm tabular-nums"), duration),
		))
	}

	// Server uptime
	uptime := time.Since(serverStartTime).Round(time.Second)
	uptimeStr := formatDuration(uptime)

	return Main(Class("container mx-auto mt-10 mb-20 px-4 max-w-4xl"),
		H1(Class("text-2xl md:text-3xl font-bold text-white mb-6"), g.Text("Debug")),

		Section(Class("bg-zinc-900/30 border border-zinc-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8 mb-6"),
			H2(Class("text-lg font-semibold text-white mb-4"), g.Text("Server")),
			Div(Class("text-sm text-zinc-400"),
				g.Text(fmt.Sprintf("Uptime: %s", uptimeStr)),
			),
		),

		Section(Class("bg-zinc-900/30 border border-zinc-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8"),
			H2(Class("text-lg font-semibold text-white mb-4"), g.Text("Poller Status")),
			Div(Class("overflow-x-auto"),
				Table(Class("w-full"),
					THead(
						Tr(Class("border-b border-zinc-700/50"),
							Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider"), g.Text("Platform")),
							Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider"), g.Text("Status")),
							Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider"), g.Text("Last Run (UTC)")),
							Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider"), g.Text("Duration")),
						),
					),
					TBody(rows...),
				),
			),
		),
	)
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func debugHandler(w http.ResponseWriter, r *http.Request) {
	PageLayout(
		"Debug - bbscope.com",
		"Debug information",
		Navbar(""),
		debugContent(),
		FooterEl(),
		"",
		true, // noindex
	).Render(w)
}
