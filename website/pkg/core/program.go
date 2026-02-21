package core

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
	g "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// programDetailHandler handles requests for /program/{platform}/{handle}
func programDetailHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/program/")
	path = strings.TrimSuffix(path, "/")

	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}

	platform, err := url.PathUnescape(parts[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	handle, err := url.PathUnescape(parts[1])
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx := context.Background()

	program, err := db.GetProgramByPlatformHandle(ctx, platform, handle)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if program == nil {
		http.NotFound(w, r)
		return
	}

	targets, err := db.ListProgramTargets(ctx, program.ID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Count in-scope and out-of-scope
	inScopeCount, oosCount := 0, 0
	for _, t := range targets {
		if t.InScope {
			inScopeCount++
		} else {
			oosCount++
		}
	}

	programURL := strings.ReplaceAll(program.URL, "api.yeswehack.com", "yeswehack.com")

	title := fmt.Sprintf("%s on %s - Bug Bounty Scope | bbscope.com", program.Handle, capitalizedPlatform(program.Platform))
	description := fmt.Sprintf("Bug bounty scope for %s on %s. %d in-scope assets, %d out-of-scope assets. View reconnaissance quick links and target details.",
		program.Handle, capitalizedPlatform(program.Platform), inScopeCount, oosCount)
	canonicalURL := fmt.Sprintf("/program/%s/%s", url.PathEscape(strings.ToLower(program.Platform)), url.PathEscape(program.Handle))

	PageLayout(
		title,
		description,
		Navbar("/scope"),
		ProgramDetailContent(program, targets, programURL, inScopeCount, oosCount),
		FooterEl(),
		canonicalURL,
		false,
	).Render(w)
}

// ProgramDetailContent renders the program detail page content.
func ProgramDetailContent(program *storage.Program, targets []storage.ProgramTarget, programURL string, inScopeCount, oosCount int) g.Node {
	var inScope, outOfScope []storage.ProgramTarget
	for _, t := range targets {
		if t.InScope {
			inScope = append(inScope, t)
		} else {
			outOfScope = append(outOfScope, t)
		}
	}

	chevronSep := Span(Class("mx-2 text-slate-600"), g.Raw(`<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>`))

	content := []g.Node{
		// Breadcrumb
		Nav(Class("flex items-center text-sm text-slate-500 mb-8"),
			A(Href("/scope"), Class("hover:text-cyan-400 transition-colors duration-200"), g.Text("Scope")),
			chevronSep,
			A(Href(fmt.Sprintf("/scope?platform=%s", strings.ToLower(program.Platform))),
				Class("hover:text-cyan-400 transition-colors duration-200"),
				g.Text(capitalizedPlatform(program.Platform)),
			),
			Span(Class("mx-2 text-slate-600"), g.Raw(`<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>`)),
			Span(Class("text-slate-200"), g.Text(program.Handle)),
		),

		// Program header
		Div(Class("flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8"),
			Div(
				H1(Class("text-2xl md:text-3xl font-bold text-white"), g.Text(program.Handle)),
				Div(Class("flex items-center gap-3 mt-2"),
					platformBadge(program.Platform),
					A(Href(programURL), Target("_blank"), Rel("noopener noreferrer"),
						Class("text-cyan-400 hover:text-cyan-300 text-sm flex items-center gap-1 transition-colors duration-200"),
						g.Text("View on "+capitalizedPlatform(program.Platform)),
						g.Raw(`<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"/></svg>`),
					),
				),
			),
			Div(Class("flex gap-4"),
				Div(Class("text-center px-5 py-3 bg-slate-800/30 border border-slate-700/50 rounded-xl"),
					Div(Class("text-2xl font-extrabold text-emerald-400 tabular-nums"), g.Text(fmt.Sprintf("%d", inScopeCount))),
					Div(Class("text-xs uppercase tracking-wider text-slate-500 mt-1 font-medium"), g.Text("In Scope")),
				),
				Div(Class("text-center px-5 py-3 bg-slate-800/30 border border-slate-700/50 rounded-xl"),
					Div(Class("text-2xl font-extrabold text-slate-400 tabular-nums"), g.Text(fmt.Sprintf("%d", oosCount))),
					Div(Class("text-xs uppercase tracking-wider text-slate-500 mt-1 font-medium"), g.Text("Out of Scope")),
				),
			),
		),
	}

	// In-scope assets table
	if len(inScope) > 0 {
		content = append(content,
			H2(Class("text-lg font-semibold text-slate-200 mb-4 flex items-center gap-2"),
				Span(Class("w-2 h-2 rounded-full bg-emerald-400")),
				g.Textf("In-Scope Assets (%d)", len(inScope)),
			),
			assetTable(inScope, true),
		)
	} else {
		content = append(content,
			H2(Class("text-lg font-semibold text-slate-200 mb-4 flex items-center gap-2"),
				Span(Class("w-2 h-2 rounded-full bg-emerald-400")),
				g.Text("In-Scope Assets"),
			),
			P(Class("text-slate-400 text-sm mb-8"), g.Text("No in-scope assets found for this program.")),
		)
	}

	// Out-of-scope section (collapsible)
	if len(outOfScope) > 0 {
		content = append(content,
			Details(Class("mt-8"),
				Summary(Class("text-lg font-semibold text-slate-200 mb-4 cursor-pointer hover:text-slate-100 transition-colors flex items-center gap-2"),
					Span(Class("w-2 h-2 rounded-full bg-slate-500")),
					g.Textf("Out-of-Scope Assets (%d)", len(outOfScope)),
				),
				Div(Class("mt-4"),
					assetTable(outOfScope, false),
				),
			),
		)
	}

	return Main(Class("container mx-auto mt-10 mb-20 px-4"),
		Section(Class("bg-slate-900/30 border border-slate-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8"),
			g.Group(content),
		),
	)
}

// assetTable renders a table of program targets with optional quick links.
func assetTable(targets []storage.ProgramTarget, showQuickLinks bool) g.Node {
	headerCols := []g.Node{
		Th(Class("px-4 py-3 text-left text-xs font-semibold text-slate-500 uppercase tracking-wider"), g.Text("Asset")),
		Th(Class("px-4 py-3 text-left text-xs font-semibold text-slate-500 uppercase tracking-wider w-28"), g.Text("Category")),
	}
	if showQuickLinks {
		headerCols = append(headerCols,
			Th(Class("px-4 py-3 text-left text-xs font-semibold text-slate-500 uppercase tracking-wider"), g.Text("Quick Links")),
		)
	}
	headerCols = append(headerCols,
		Th(Class("px-4 py-3 text-left text-xs font-semibold text-slate-500 uppercase tracking-wider w-16"), g.Text("")),
	)

	var rows []g.Node
	for i, t := range targets {
		category := strings.ToUpper(scope.NormalizeCategory(t.Category))
		rowBg := ""
		if i%2 == 1 {
			rowBg = " bg-slate-800/20"
		}

		cols := []g.Node{
			Td(Class("px-4 py-3 text-sm text-slate-200 break-all"),
				assetDisplay(t),
			),
			Td(Class("px-4 py-3 text-sm"),
				categoryBadge(category),
			),
		}
		if showQuickLinks {
			cols = append(cols,
				Td(Class("px-4 py-3 text-sm"),
					quickLinksForAsset(t.TargetDisplay, category),
				),
			)
		}
		cols = append(cols,
			Td(Class("px-4 py-3 text-sm"),
				copyButton(t.TargetDisplay),
			),
		)

		rows = append(rows, Tr(Class("border-b border-slate-800/50 hover:bg-slate-800/50 transition-colors duration-150"+rowBg),
			g.Group(cols),
		))
	}

	return Div(Class("overflow-x-auto rounded-xl border border-slate-700/50 mb-6"),
		Table(Class("min-w-full divide-y divide-slate-700"),
			THead(Class("bg-slate-800/80"),
				Tr(g.Group(headerCols)),
			),
			TBody(Class("bg-slate-900/50 divide-y divide-slate-800"),
				g.Group(rows),
			),
		),
	)
}

// assetDisplay renders the asset value - as a clickable link if it looks like a URL/domain.
func assetDisplay(t storage.ProgramTarget) g.Node {
	display := t.TargetDisplay
	if display == "" {
		display = t.TargetRaw
	}

	cat := strings.ToLower(scope.NormalizeCategory(t.Category))
	if cat == "url" || cat == "wildcard" || cat == "api" {
		href := display
		if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
			href = strings.TrimPrefix(href, "*.")
			href = "https://" + href
		}
		return A(Href(href), Target("_blank"), Rel("noopener noreferrer"),
			Class("text-cyan-400 hover:text-cyan-300 hover:underline transition-colors"),
			g.Text(display),
		)
	}

	return Span(g.Text(display))
}

// categoryBadge renders a colored badge for the asset category.
func categoryBadge(category string) g.Node {
	colors := "bg-slate-700 text-slate-300"
	switch strings.ToLower(category) {
	case "url":
		colors = "bg-blue-900/50 text-blue-300 border border-blue-800"
	case "wildcard":
		colors = "bg-purple-900/50 text-purple-300 border border-purple-800"
	case "cidr":
		colors = "bg-amber-900/50 text-amber-300 border border-amber-800"
	case "android":
		colors = "bg-green-900/50 text-green-300 border border-green-800"
	case "ios":
		colors = "bg-gray-900/50 text-gray-300 border border-gray-600"
	case "api":
		colors = "bg-cyan-900/50 text-cyan-300 border border-cyan-800"
	case "other":
		colors = "bg-slate-800 text-slate-400 border border-slate-700"
	case "hardware":
		colors = "bg-orange-900/50 text-orange-300 border border-orange-800"
	case "executable":
		colors = "bg-red-900/50 text-red-300 border border-red-800"
	}
	return Span(Class("inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md "+colors), g.Text(category))
}

// platformBadge renders a colored badge for the platform name.
func platformBadge(platform string) g.Node {
	colors := "bg-slate-700 text-slate-300"
	switch strings.ToLower(platform) {
	case "h1", "hackerone":
		colors = "bg-blue-900/50 text-blue-300 border border-blue-800"
	case "bc", "bugcrowd":
		colors = "bg-emerald-900/50 text-emerald-300 border border-emerald-800"
	case "it", "intigriti":
		colors = "bg-purple-900/50 text-purple-300 border border-purple-800"
	case "ywh", "yeswehack":
		colors = "bg-yellow-900/50 text-yellow-300 border border-yellow-800"
	}
	return Span(Class("inline-flex items-center px-2.5 py-0.5 text-[11px] font-semibold rounded-md "+colors), g.Text(capitalizedPlatform(platform)))
}

// capitalizedPlatform returns a properly capitalized platform name.
func capitalizedPlatform(platform string) string {
	switch strings.ToLower(platform) {
	case "h1", "hackerone":
		return "HackerOne"
	case "bc", "bugcrowd":
		return "Bugcrowd"
	case "it", "intigriti":
		return "Intigriti"
	case "ywh", "yeswehack":
		return "YesWeHack"
	default:
		return platform
	}
}

// QuickLink represents a reconnaissance tool link for an asset.
type QuickLink struct {
	Label       string
	URL         string
	Description string
}

// quickLinksForAsset generates contextual reconnaissance tool links based on asset category and value.
func quickLinksForAsset(target, category string) g.Node {
	links := generateQuickLinks(target, category)
	if len(links) == 0 {
		return Span(Class("text-slate-500 text-xs"), g.Text("-"))
	}

	var nodes []g.Node
	for _, link := range links {
		nodes = append(nodes,
			A(Href(link.URL), Target("_blank"), Rel("noopener noreferrer"),
				Class("inline-flex items-center px-2 py-0.5 text-[11px] rounded-md bg-slate-800/50 text-slate-400 hover:bg-slate-700 hover:text-cyan-400 border border-slate-700/50 transition-all duration-200"),
				g.Attr("title", link.Description),
				g.Text(link.Label),
			),
		)
	}
	return Div(Class("flex flex-wrap gap-1"), g.Group(nodes))
}

// generateQuickLinks produces quick links for a given target and category.
func generateQuickLinks(target, category string) []QuickLink {
	cat := strings.ToLower(category)
	domain := extractDomain(target)

	switch cat {
	case "url", "wildcard", "api":
		if domain == "" {
			return nil
		}
		return []QuickLink{
			{Label: "crt.sh", URL: fmt.Sprintf("https://crt.sh/?q=%%25.%s", url.QueryEscape(domain)), Description: "TLS certificate transparency search"},
			{Label: "Google", URL: fmt.Sprintf("https://www.google.com/search?q=site:%s", url.QueryEscape(domain)), Description: "Google dorking - site search"},
			{Label: "Shodan", URL: fmt.Sprintf("https://www.shodan.io/search?query=hostname:%s", url.QueryEscape(domain)), Description: "Shodan host search"},
			{Label: "SecurityTrails", URL: fmt.Sprintf("https://securitytrails.com/domain/%s/dns", url.QueryEscape(domain)), Description: "DNS and subdomain history"},
			{Label: "Wayback", URL: fmt.Sprintf("https://web.archive.org/web/*/%s", url.QueryEscape(target)), Description: "Wayback Machine archives"},
			{Label: "DNSdumpster", URL: fmt.Sprintf("https://dnsdumpster.com/?search=%s", url.QueryEscape(domain)), Description: "DNS recon and subdomain discovery"},
			{Label: "VirusTotal", URL: fmt.Sprintf("https://www.virustotal.com/gui/domain/%s", url.QueryEscape(domain)), Description: "VirusTotal domain analysis"},
		}
	case "cidr":
		return []QuickLink{
			{Label: "Shodan", URL: fmt.Sprintf("https://www.shodan.io/search?query=net:%s", url.QueryEscape(target)), Description: "Shodan network search"},
			{Label: "Censys", URL: fmt.Sprintf("https://search.censys.io/hosts?q=%s", url.QueryEscape(target)), Description: "Censys host search"},
		}
	case "android":
		pkg := extractPackageName(target)
		if pkg != "" {
			return []QuickLink{
				{Label: "Play Store", URL: fmt.Sprintf("https://play.google.com/store/apps/details?id=%s", url.QueryEscape(pkg)), Description: "Google Play Store listing"},
			}
		}
	case "ios":
		// iOS apps are hard to link directly
		return nil
	}

	return nil
}

// extractDomain extracts the root domain from a target string.
func extractDomain(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}

	// Handle wildcards: *.example.com -> example.com
	target = strings.TrimPrefix(target, "*.")

	// Try URL parsing if it has a scheme
	if strings.Contains(target, "://") {
		if u, err := url.Parse(target); err == nil && u.Host != "" {
			host := u.Hostname()
			if host != "" {
				return host
			}
		}
	}

	// Try adding a scheme and parsing
	if u, err := url.Parse("https://" + target); err == nil && u.Host != "" {
		host := u.Hostname()
		// Verify it looks like a domain (has a dot, no spaces)
		if strings.Contains(host, ".") && !strings.Contains(host, " ") {
			return host
		}
	}

	// If it looks like a bare domain
	cleaned := strings.Split(target, "/")[0]
	cleaned = strings.Split(cleaned, ":")[0] // Remove port
	if strings.Contains(cleaned, ".") && !strings.Contains(cleaned, " ") {
		return cleaned
	}

	return ""
}

// extractPackageName extracts an Android package name from a target string.
func extractPackageName(target string) string {
	target = strings.TrimSpace(target)

	// If it's a Play Store URL, extract the package ID
	if strings.Contains(target, "play.google.com") {
		if u, err := url.Parse(target); err == nil {
			id := u.Query().Get("id")
			if id != "" {
				return id
			}
		}
	}

	// If it looks like a package name (e.g., com.example.app)
	if strings.Contains(target, ".") && !strings.Contains(target, "/") && !strings.Contains(target, " ") {
		parts := strings.Split(target, ".")
		if len(parts) >= 2 {
			return target
		}
	}

	return ""
}

// copyButton renders a button that copies text to clipboard.
func copyButton(text string) g.Node {
	// Use JSON-safe escaping for the text in the onclick handler
	escaped := strings.ReplaceAll(text, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)

	return Button(
		Type("button"),
		Class("p-1.5 text-slate-500 hover:text-cyan-400 transition-all duration-200 rounded-md hover:bg-slate-800/50"),
		g.Attr("onclick", fmt.Sprintf("navigator.clipboard.writeText('%s').then(()=>{this.textContent='Copied!';setTimeout(()=>this.textContent='Copy',1500)})", escaped)),
		g.Attr("title", "Copy to clipboard"),
		g.Text("Copy"),
	)
}
