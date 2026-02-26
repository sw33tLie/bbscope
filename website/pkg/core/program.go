package core

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"time"

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

	// Try active programs first, then fall back to disabled/ignored
	program, err := db.GetProgramByPlatformHandle(ctx, platform, handle)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if program == nil {
		program, err = db.GetProgramByPlatformHandleAny(ctx, platform, handle)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
	if program == nil {
		http.NotFound(w, r)
		return
	}

	targets, err := db.ListProgramTargets(ctx, program.ID, true)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// For removed programs with no targets in targets_raw, reconstruct from scope_changes history
	if len(targets) == 0 && program.Disabled {
		targets, err = db.ListProgramTargetsFromHistory(ctx, program.Platform, program.Handle)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	// Count in-scope and out-of-scope; derive isBBP
	inScopeCount, oosCount := 0, 0
	isBBP := false
	for _, t := range targets {
		if t.InScope {
			inScopeCount++
		} else {
			oosCount++
		}
		if t.IsBBP {
			isBBP = true
		}
	}

	// Fetch recent scope changes for this program
	changes, err := db.ListProgramChanges(ctx, program.Platform, program.Handle, 100)
	if err != nil {
		changes = nil // non-fatal, just skip the section
	}

	programURL := strings.ReplaceAll(program.URL, "api.yeswehack.com", "yeswehack.com")

	title := fmt.Sprintf("%s on %s - Bug Bounty Scope | bbscope.com", program.Handle, capitalizedPlatform(program.Platform))
	description := buildProgramDescription(program, targets, inScopeCount, isBBP)
	canonicalURL := fmt.Sprintf("/program/%s/%s", url.PathEscape(strings.ToLower(program.Platform)), url.PathEscape(program.Handle))

	PageLayout(
		title,
		description,
		Navbar("/scope"),
		ProgramDetailContent(program, targets, changes, programURL, inScopeCount, oosCount, isBBP),
		FooterEl(),
		canonicalURL,
		false,
	).Render(w)
}

// ProgramDetailContent renders the program detail page content.
func ProgramDetailContent(program *storage.Program, targets []storage.ProgramTarget, changes []storage.Change, programURL string, inScopeCount, oosCount int, isBBP bool) g.Node {
	var inScope, outOfScope []storage.ProgramTarget
	for _, t := range targets {
		if t.InScope {
			inScope = append(inScope, t)
		} else {
			outOfScope = append(outOfScope, t)
		}
	}

	chevronSep := Span(Class("mx-2 text-zinc-600"), g.Raw(`<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>`))

	content := []g.Node{
		// Breadcrumb
		Nav(Class("flex items-center text-sm text-zinc-500 mb-8"),
			A(Href("/scope"), Class("hover:text-cyan-400 transition-colors duration-200"), g.Text("Scope")),
			chevronSep,
			A(Href(fmt.Sprintf("/scope?platform=%s", strings.ToLower(program.Platform))),
				Class("hover:text-cyan-400 transition-colors duration-200"),
				g.Text(capitalizedPlatform(program.Platform)),
			),
			Span(Class("mx-2 text-zinc-600"), g.Raw(`<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>`)),
			Span(Class("text-zinc-200"), g.Text(program.Handle)),
		),

		// Program removed banner
		g.If(program.Disabled,
			Div(Class("mb-8 rounded-xl border border-red-800/50 bg-red-900/20 px-5 py-4 flex items-start gap-3"),
				Div(Class("flex-shrink-0 mt-0.5"),
					g.Raw(`<svg class="w-5 h-5 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/></svg>`),
				),
				Div(
					Div(Class("font-semibold text-red-300 text-sm"), g.Text("Program Removed")),
					P(Class("text-red-200/70 text-sm mt-1 leading-relaxed"), g.Text("This program is no longer available on "+capitalizedPlatform(program.Platform)+". The scope data shown below is historical and may not reflect the final state of the program.")),
				),
			),
		),

		// VDP info banner
		g.If(!isBBP,
			Div(Class("mb-8 rounded-xl border border-amber-800/50 bg-amber-900/20 px-5 py-4 flex items-start gap-3"),
				// Info icon
				Div(Class("flex-shrink-0 mt-0.5"),
					g.Raw(`<svg class="w-5 h-5 text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>`),
				),
				Div(
					Div(Class("font-semibold text-amber-300 text-sm"), g.Text("Vulnerability Disclosure Program (VDP)")),
					P(Class("text-amber-200/70 text-sm mt-1 leading-relaxed"), g.Text("VDPs are meant for responsibly reporting vulnerabilities you encounter — not for actively hunting for fame or reputation. Even if you're just starting out, consider focusing on rewarded bug bounty programs instead.")),
				),
			),
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
					// Data source toggle (Raw / AI Enhanced)
					Div(
						ID("detail-ai-toggle"),
						Class("inline-flex rounded-lg border border-zinc-700/50 overflow-hidden bg-zinc-800/80"),
						g.Attr("data-platform", program.Platform),
						g.Attr("data-handle", program.Handle),
						Span(
							ID("detail-toggle-raw"),
							Class("px-3 py-1 text-xs font-medium cursor-pointer transition-all duration-200 bg-cyan-500 text-white"),
							g.Text("Raw"),
						),
						Span(
							ID("detail-toggle-ai"),
							Class("px-3 py-1 text-xs font-medium cursor-pointer transition-all duration-200 flex items-center gap-1 text-zinc-400 hover:text-zinc-200"),
							g.Raw(`<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"/></svg>`),
							g.Text("AI Enhanced"),
						),
					),
				),
			),
			Div(Class("flex gap-4"),
				Div(Class("text-center px-5 py-3 bg-zinc-800/30 border border-zinc-700/50 rounded-xl"),
					Div(ID("in-scope-count"), Class("text-2xl font-extrabold text-emerald-400 tabular-nums"), g.Text(fmt.Sprintf("%d", inScopeCount))),
					Div(Class("text-xs uppercase tracking-wider text-zinc-500 mt-1 font-medium"), g.Text("In Scope")),
				),
				Div(Class("text-center px-5 py-3 bg-zinc-800/30 border border-zinc-700/50 rounded-xl"),
					Div(ID("out-scope-count"), Class("text-2xl font-extrabold text-zinc-400 tabular-nums"), g.Text(fmt.Sprintf("%d", oosCount))),
					Div(Class("text-xs uppercase tracking-wider text-zinc-500 mt-1 font-medium"), g.Text("Out of Scope")),
				),
			),
		),
	}

	// Scope tables container (swapped by JS on AI toggle)
	scopeTables := scopeTablesNode(inScope, outOfScope)
	content = append(content, Div(ID("scope-tables-container"), scopeTables))

	// AI toggle script
	content = append(content, programDetailAIToggleScript())

	// Scope changes section
	if len(changes) > 0 {
		content = append(content, scopeChangesSection(changes))
	}

	return Main(Class("container mx-auto mt-10 mb-20 px-4"),
		Section(Class("bg-zinc-900/30 border border-zinc-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8"),
			g.Group(content),
		),
	)
}

// scopeTablesNode renders the in-scope and out-of-scope table sections.
func scopeTablesNode(inScope, outOfScope []storage.ProgramTarget) g.Node {
	var nodes []g.Node

	if len(inScope) > 0 {
		nodes = append(nodes,
			Details(Class(""), g.Attr("open", ""),
				Summary(Class("text-lg font-semibold text-zinc-200 mb-4 cursor-pointer hover:text-zinc-100 transition-colors flex items-center gap-2"),
					Span(Class("w-2 h-2 rounded-full bg-emerald-400")),
					g.Textf("In-Scope Assets (%d)", len(inScope)),
				),
				Div(Class("mt-2"),
					assetTable(inScope, true),
				),
			),
		)
	} else {
		nodes = append(nodes,
			H2(Class("text-lg font-semibold text-zinc-200 mb-4 flex items-center gap-2"),
				Span(Class("w-2 h-2 rounded-full bg-emerald-400")),
				g.Text("In-Scope Assets"),
			),
			P(Class("text-zinc-400 text-sm mb-8"), g.Text("No in-scope assets found for this program.")),
		)
	}

	if len(outOfScope) > 0 {
		nodes = append(nodes,
			Details(Class("mt-8"),
				Summary(Class("text-lg font-semibold text-zinc-200 mb-4 cursor-pointer hover:text-zinc-100 transition-colors flex items-center gap-2"),
					Span(Class("w-2 h-2 rounded-full bg-zinc-500")),
					g.Textf("Out-of-Scope Assets (%d)", len(outOfScope)),
				),
				Div(Class("mt-4"),
					assetTable(outOfScope, false),
				),
			),
		)
	}

	return g.Group(nodes)
}

// scopeChangesSection renders the collapsible scope changes timeline for a program.
func scopeChangesSection(changes []storage.Change) g.Node {
	// Group changes by date for the timeline
	type dayGroup struct {
		date    string
		changes []storage.Change
	}
	var groups []dayGroup
	var currentDate string
	for _, c := range changes {
		d := c.OccurredAt.Format("2006-01-02")
		if d != currentDate {
			groups = append(groups, dayGroup{date: d})
			currentDate = d
		}
		groups[len(groups)-1].changes = append(groups[len(groups)-1].changes, c)
	}

	var timelineNodes []g.Node
	for _, group := range groups {
		// Parse date for display
		t, _ := time.Parse("2006-01-02", group.date)
		dateLabel := t.Format("Jan 2, 2006")

		var rows []g.Node
		for i, c := range group.changes {
			rowBg := ""
			if i%2 == 1 {
				rowBg = " bg-zinc-800/20"
			}

			var changeType string
			if c.Category == "program" {
				if c.ChangeType == "added" {
					changeType = "program_added"
				} else {
					changeType = "program_removed"
				}
			} else {
				if c.ChangeType == "added" {
					changeType = "asset_added"
				} else {
					changeType = "asset_removed"
				}
			}

			target := c.TargetNormalized
			if target == "" {
				target = c.TargetRaw
			}
			category := strings.ToUpper(scope.NormalizeCategory(c.Category))

			isProgramLevel := c.Category == "program"

			var scopeNode g.Node
			if isProgramLevel {
				scopeNode = Span(Class("text-zinc-500"), g.Text("—"))
			} else if c.InScope {
				scopeNode = Span(Class("inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md bg-emerald-900/50 text-emerald-300 border border-emerald-800"), g.Text("In Scope"))
			} else {
				scopeNode = Span(Class("inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md bg-zinc-800 text-zinc-400 border border-zinc-700"), g.Text("Out of Scope"))
			}

			rows = append(rows,
				Tr(Class("border-b border-zinc-800/50 hover:bg-zinc-800/50 transition-colors duration-150"+rowBg),
					Td(Class("px-4 py-2.5 text-sm"), changeTypeBadge(changeType)),
					Td(Class("px-4 py-2.5 text-sm text-zinc-200 break-all"),
						g.If(isProgramLevel, Span(Class("text-zinc-500"), g.Text("—"))),
						g.If(!isProgramLevel, g.Text(target)),
					),
					Td(Class("px-4 py-2.5 text-sm"),
						g.If(isProgramLevel, Span(Class("text-zinc-500"), g.Text("—"))),
						g.If(!isProgramLevel, categoryBadge(category)),
					),
					Td(Class("px-4 py-2.5 text-sm"), scopeNode),
					Td(Class("px-4 py-2.5 text-sm text-zinc-500 whitespace-nowrap"), g.Text(c.OccurredAt.Format("15:04"))),
				),
			)
		}

		timelineNodes = append(timelineNodes,
			Div(Class("mb-4"),
				Div(Class("text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-2 px-1"), g.Text(dateLabel)),
				Div(Class("overflow-x-auto rounded-lg border border-zinc-700/50"),
					Table(Class("min-w-full divide-y divide-zinc-700"),
						THead(Class("bg-zinc-800/80"),
							Tr(
								Th(Class("px-4 py-2 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-32"), g.Text("Change")),
								Th(Class("px-4 py-2 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider"), g.Text("Asset")),
								Th(Class("px-4 py-2 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-28"), g.Text("Category")),
								Th(Class("px-4 py-2 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-28"), g.Text("Scope")),
								Th(Class("px-4 py-2 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-16"), g.Text("Time")),
							),
						),
						TBody(Class("bg-zinc-900/50 divide-y divide-zinc-800"),
							g.Group(rows),
						),
					),
				),
			),
		)
	}

	return Details(Class("mt-10"),
		Summary(Class("text-lg font-semibold text-zinc-200 mb-4 cursor-pointer hover:text-zinc-100 transition-colors flex items-center gap-2"),
			g.Raw(`<svg class="w-4 h-4 text-zinc-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>`),
			g.Textf("Scope Changes (%d)", len(changes)),
		),
		Div(Class("mt-2"),
			g.Group(timelineNodes),
		),
	)
}

// programDetailAIToggleScript returns an inline script for toggling AI/raw data on program detail pages.
func programDetailAIToggleScript() g.Node {
	return Script(g.Raw(`
(function(){
  var wrapper = document.getElementById('detail-ai-toggle');
  if (!wrapper) return;
  var platform = wrapper.getAttribute('data-platform');
  var handle = wrapper.getAttribute('data-handle');
  var rawBtn = document.getElementById('detail-toggle-raw');
  var aiBtn = document.getElementById('detail-toggle-ai');
  var aiMode = false;
  var cache = {};

  function escapeHtml(s) {
    var d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
  }

  function normalizeCategory(cat) {
    return (cat || 'other').toUpperCase();
  }

  function categoryBadgeClass(cat) {
    switch (cat.toLowerCase()) {
      case 'url': return 'bg-blue-900/50 text-blue-300 border border-blue-800';
      case 'wildcard': return 'bg-purple-900/50 text-purple-300 border border-purple-800';
      case 'cidr': return 'bg-amber-900/50 text-amber-300 border border-amber-800';
      case 'android': return 'bg-green-900/50 text-green-300 border border-green-800';
      case 'ios': return 'bg-gray-900/50 text-gray-300 border border-gray-600';
      case 'api': return 'bg-cyan-900/50 text-cyan-300 border border-cyan-800';
      case 'other': return 'bg-zinc-800 text-zinc-400 border border-zinc-700';
      case 'hardware': return 'bg-orange-900/50 text-orange-300 border border-orange-800';
      case 'executable': return 'bg-red-900/50 text-red-300 border border-red-800';
      default: return 'bg-zinc-700 text-zinc-300';
    }
  }

  function assetLink(target, cat) {
    var c = cat.toLowerCase();
    if (c === 'url' || c === 'wildcard' || c === 'api') {
      var href = target;
      if (href.indexOf('http://') !== 0 && href.indexOf('https://') !== 0) {
        href = href.replace(/^\*\./, '');
        href = 'https://' + href;
      }
      return '<a href="' + escapeHtml(href) + '" target="_blank" rel="noopener noreferrer" class="text-cyan-400 hover:text-cyan-300 hover:underline transition-colors">' + escapeHtml(target) + '</a>';
    }
    return '<span>' + escapeHtml(target) + '</span>';
  }

  function extractDomain(target) {
    var t = target.replace(/^\*\./, '');
    if (t.indexOf('://') !== -1) {
      try { var u = new URL(t); if (u.hostname) return u.hostname; } catch(e) {}
    }
    try { var u = new URL('https://' + t); if (u.hostname && u.hostname.indexOf('.') !== -1 && u.hostname.indexOf(' ') === -1) return u.hostname; } catch(e) {}
    var cleaned = t.split('/')[0].split(':')[0];
    if (cleaned.indexOf('.') !== -1 && cleaned.indexOf(' ') === -1) return cleaned;
    return '';
  }

  function extractPackageName(target) {
    if (target.indexOf('play.google.com') !== -1) {
      try { var u = new URL(target); var id = u.searchParams.get('id'); if (id) return id; } catch(e) {}
    }
    if (target.indexOf('.') !== -1 && target.indexOf('/') === -1 && target.indexOf(' ') === -1 && target.split('.').length >= 2) return target;
    return '';
  }

  var linkCls = 'inline-flex items-center px-2 py-0.5 text-[11px] rounded-md bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-cyan-400 border border-zinc-700/50 transition-all duration-200';

  function quickLinksHtml(target, cat) {
    var c = cat.toLowerCase();
    var domain = extractDomain(target);
    var links = [];
    if ((c === 'url' || c === 'wildcard' || c === 'api') && domain) {
      links.push({l:'crt.sh', u:'https://crt.sh/?q=%25.' + encodeURIComponent(domain), d:'TLS certificate transparency search'});
      links.push({l:'Google', u:'https://www.google.com/search?q=site:' + encodeURIComponent(domain), d:'Google dorking - site search'});
      links.push({l:'Shodan', u:'https://www.shodan.io/search?query=hostname:' + encodeURIComponent(domain), d:'Shodan host search'});
      links.push({l:'SecurityTrails', u:'https://securitytrails.com/domain/' + encodeURIComponent(domain) + '/dns', d:'DNS and subdomain history'});
      links.push({l:'Wayback', u:'https://web.archive.org/web/*/' + target, d:'Wayback Machine archives'});
      links.push({l:'DNSdumpster', u:'https://dnsdumpster.com/?search=' + encodeURIComponent(domain), d:'DNS recon and subdomain discovery'});
      links.push({l:'VirusTotal', u:'https://www.virustotal.com/gui/domain/' + encodeURIComponent(domain), d:'VirusTotal domain analysis'});
      links.push({l:'AlienVault OTX', u:'https://otx.alienvault.com/indicator/domain/' + encodeURIComponent(domain), d:'AlienVault OTX threat intelligence'});
      links.push({l:'GitHub', u:'https://github.com/search?q=' + encodeURIComponent(domain) + '&type=code', d:'GitHub code search'});
      links.push({l:'Fofa', u:'https://en.fofa.info/result?qbase64=' + btoa('domain="' + domain + '"'), d:'Fofa asset search'});
    } else if (c === 'cidr') {
      links.push({l:'Shodan', u:'https://www.shodan.io/search?query=net:' + encodeURIComponent(target), d:'Shodan network search'});
      links.push({l:'Censys', u:'https://search.censys.io/hosts?q=' + encodeURIComponent(target), d:'Censys host search'});
      links.push({l:'BGP HE', u:'https://bgp.he.net/net/' + target, d:'Hurricane Electric BGP prefix info'});
    } else if (c === 'android') {
      var pkg = extractPackageName(target);
      if (pkg) {
        links.push({l:'Play Store', u:'https://play.google.com/store/apps/details?id=' + encodeURIComponent(pkg), d:'Google Play Store listing'});
        links.push({l:'APKPure', u:'https://apkpure.com/search?q=' + encodeURIComponent(pkg), d:'APKPure app download'});
        links.push({l:'APKMirror', u:'https://www.apkmirror.com/?post_type=app_release&searchtype=apk&s=' + encodeURIComponent(pkg), d:'APKMirror app download'});
      }
    }
    if (links.length === 0) return '<span class="text-zinc-500 text-xs">-</span>';
    var html = '<div class="flex flex-wrap gap-1">';
    for (var i = 0; i < links.length; i++) {
      html += '<a href="' + escapeHtml(links[i].u) + '" target="_blank" rel="noopener noreferrer" class="' + linkCls + '" title="' + escapeHtml(links[i].d) + '">' + escapeHtml(links[i].l) + '</a>';
    }
    html += '</div>';
    return html;
  }

  function renderTable(targets, showLinks) {
    if (!targets || targets.length === 0) return '';
    var html = '<div class="overflow-x-auto rounded-xl border border-zinc-700/50 mb-6"><table class="min-w-full divide-y divide-zinc-700"><thead class="bg-zinc-800/80"><tr>';
    html += '<th class="px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider">Asset</th>';
    html += '<th class="px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-28">Category</th>';
    html += '<th class="px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-24">Bounty</th>';
    if (showLinks) html += '<th class="px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider">Quick Links</th>';
    html += '<th class="px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-16"></th>';
    html += '</tr></thead><tbody class="bg-zinc-900/50 divide-y divide-zinc-800">';
    for (var i = 0; i < targets.length; i++) {
      var t = targets[i];
      var display = t.target || t.target_raw || '';
      var cat = normalizeCategory(t.category);
      var rowBg = i % 2 === 1 ? ' bg-zinc-800/20' : '';
      html += '<tr class="border-b border-zinc-800/50 hover:bg-zinc-800/50 transition-colors duration-150' + rowBg + '">';
      html += '<td class="px-4 py-3 text-sm text-zinc-200 break-all">' + assetLink(display, cat) + '</td>';
      html += '<td class="px-4 py-3 text-sm"><span class="inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md ' + categoryBadgeClass(cat) + '">' + escapeHtml(cat) + '</span></td>';
      var bountyBadge = t.is_bbp
        ? '<span class="inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md bg-emerald-900/50 text-emerald-300 border border-emerald-800">Yes</span>'
        : '<span class="inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md bg-red-900/50 text-red-300 border border-red-800">No</span>';
      html += '<td class="px-4 py-3 text-sm">' + bountyBadge + '</td>';
      if (showLinks) html += '<td class="px-4 py-3 text-sm">' + quickLinksHtml(display, cat) + '</td>';
      var esc = display.replace(/\\/g,'\\\\').replace(/'/g,"\\'").replace(/\n/g,'\\n');
      html += '<td class="px-4 py-3 text-sm"><button type="button" class="p-1.5 text-zinc-500 hover:text-cyan-400 transition-all duration-200 rounded-md hover:bg-zinc-800/50" onclick="navigator.clipboard.writeText(\'' + esc + '\').then(()=>{this.textContent=\'Copied!\';setTimeout(()=>this.textContent=\'Copy\',1500)})" title="Copy to clipboard">Copy</button></td>';
      html += '</tr>';
    }
    html += '</tbody></table></div>';
    return html;
  }

  function renderScopeTables(data) {
    var inScope = data.in_scope || [];
    var outScope = data.out_of_scope || [];
    var html = '';

    // Update counts
    var isc = document.getElementById('in-scope-count');
    var osc = document.getElementById('out-scope-count');
    if (isc) isc.textContent = inScope.length;
    if (osc) osc.textContent = outScope.length;

    if (inScope.length > 0) {
      html += '<details open><summary class="text-lg font-semibold text-zinc-200 mb-4 cursor-pointer hover:text-zinc-100 transition-colors flex items-center gap-2"><span class="w-2 h-2 rounded-full bg-emerald-400"></span>In-Scope Assets (' + inScope.length + ')</summary><div class="mt-2">';
      html += renderTable(inScope, true);
      html += '</div></details>';
    } else {
      html += '<h2 class="text-lg font-semibold text-zinc-200 mb-4 flex items-center gap-2"><span class="w-2 h-2 rounded-full bg-emerald-400"></span>In-Scope Assets</h2>';
      html += '<p class="text-zinc-400 text-sm mb-8">No in-scope assets found for this program.</p>';
    }

    if (outScope.length > 0) {
      html += '<details class="mt-8"><summary class="text-lg font-semibold text-zinc-200 mb-4 cursor-pointer hover:text-zinc-100 transition-colors flex items-center gap-2"><span class="w-2 h-2 rounded-full bg-zinc-500"></span>Out-of-Scope Assets (' + outScope.length + ')</summary><div class="mt-4">';
      html += renderTable(outScope, false);
      html += '</div></details>';
    }

    return html;
  }

  var activeClass = 'px-3 py-1 text-xs font-medium cursor-pointer transition-all duration-200 bg-cyan-500 text-white';
  var inactiveClass = 'px-3 py-1 text-xs font-medium cursor-pointer transition-all duration-200 text-zinc-400 hover:text-zinc-200';
  var inactiveAIClass = inactiveClass + ' flex items-center gap-1';
  var activeAIClass = activeClass + ' flex items-center gap-1';

  function syncToggle() {
    if (aiMode) {
      rawBtn.className = inactiveClass;
      aiBtn.className = activeAIClass;
    } else {
      rawBtn.className = activeClass;
      aiBtn.className = inactiveAIClass;
    }
  }

  function fetchAndRender() {
    var key = aiMode ? 'ai' : 'raw';
    if (cache[key]) {
      document.getElementById('scope-tables-container').innerHTML = renderScopeTables(cache[key]);
      return;
    }
    var url = '/api/v1/programs/' + encodeURIComponent(platform) + '/' + encodeURIComponent(handle);
    if (!aiMode) url += '?raw=true';
    fetch(url).then(function(r){ return r.json(); }).then(function(data){
      cache[key] = data;
      document.getElementById('scope-tables-container').innerHTML = renderScopeTables(data);
    });
  }

  // Prefetch the other mode's data so toggling is instant
  (function prefetch() {
    var otherUrl = '/api/v1/programs/' + encodeURIComponent(platform) + '/' + encodeURIComponent(handle);
    if (aiMode) otherUrl += '?raw=true';
    fetch(otherUrl).then(function(r){ return r.json(); }).then(function(data){
      cache[aiMode ? 'raw' : 'ai'] = data;
    });
  })();

  rawBtn.addEventListener('click', function(){
    if (!aiMode) return;
    aiMode = false;
    syncToggle();
    fetchAndRender();
  });
  aiBtn.addEventListener('click', function(){
    if (aiMode) return;
    aiMode = true;
    syncToggle();
    fetchAndRender();
  });
})();
`))
}

// assetTable renders a table of program targets with optional quick links.
func assetTable(targets []storage.ProgramTarget, showQuickLinks bool) g.Node {
	headerCols := []g.Node{
		Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider"), g.Text("Asset")),
		Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-28"), g.Text("Category")),
		Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-24"), g.Text("Bounty")),
	}
	if showQuickLinks {
		headerCols = append(headerCols,
			Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider"), g.Text("Quick Links")),
		)
	}
	headerCols = append(headerCols,
		Th(Class("px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-16"), g.Text("")),
	)

	var rows []g.Node
	for i, t := range targets {
		category := strings.ToUpper(scope.NormalizeCategory(t.Category))
		rowBg := ""
		if i%2 == 1 {
			rowBg = " bg-zinc-800/20"
		}

		var bountyBadge g.Node
		if t.IsBBP {
			bountyBadge = Span(Class("inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md bg-emerald-900/50 text-emerald-300 border border-emerald-800"), g.Text("Yes"))
		} else {
			bountyBadge = Span(Class("inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md bg-red-900/50 text-red-300 border border-red-800"), g.Text("No"))
		}

		cols := []g.Node{
			Td(Class("px-4 py-3 text-sm text-zinc-200 break-all"),
				assetDisplay(t),
			),
			Td(Class("px-4 py-3 text-sm"),
				categoryBadge(category),
			),
			Td(Class("px-4 py-3 text-sm"),
				bountyBadge,
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

		rows = append(rows, Tr(Class("border-b border-zinc-800/50 hover:bg-zinc-800/50 transition-colors duration-150"+rowBg),
			g.Group(cols),
		))
	}

	return Div(Class("overflow-x-auto rounded-xl border border-zinc-700/50 mb-6"),
		Table(Class("min-w-full divide-y divide-zinc-700"),
			THead(Class("bg-zinc-800/80"),
				Tr(g.Group(headerCols)),
			),
			TBody(Class("bg-zinc-900/50 divide-y divide-zinc-800"),
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
	colors := "bg-zinc-700 text-zinc-300"
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
		colors = "bg-zinc-800 text-zinc-400 border border-zinc-700"
	case "hardware":
		colors = "bg-orange-900/50 text-orange-300 border border-orange-800"
	case "executable":
		colors = "bg-red-900/50 text-red-300 border border-red-800"
	}
	return Span(Class("inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md "+colors), g.Text(category))
}

// platformBadge renders a colored badge for the platform name.
func platformBadge(platform string) g.Node {
	colors := "bg-zinc-700 text-zinc-300"
	switch strings.ToLower(platform) {
	case "h1", "hackerone":
		colors = "bg-blue-900/50 text-blue-300 border border-blue-800"
	case "bc", "bugcrowd":
		colors = "bg-orange-900/50 text-orange-300 border border-orange-800"
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
		return Span(Class("text-zinc-500 text-xs"), g.Text("-"))
	}

	var nodes []g.Node
	for _, link := range links {
		nodes = append(nodes,
			A(Href(link.URL), Target("_blank"), Rel("noopener noreferrer"),
				Class("inline-flex items-center px-2 py-0.5 text-[11px] rounded-md bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-cyan-400 border border-zinc-700/50 transition-all duration-200"),
				g.Attr("title", link.Description),
				g.Text(link.Label),
			),
		)
	}
	return Div(Class("flex flex-wrap gap-1"), g.Group(nodes))
}

// buildProgramDescription builds a unique meta description including actual in-scope targets.
func buildProgramDescription(program *storage.Program, targets []storage.ProgramTarget, inScopeCount int, isBBP bool) string {
	programType := "VDP"
	if isBBP {
		programType = "BBP"
	}

	// Collect in-scope target names (prefer domains/wildcards over long descriptions)
	var assetNames []string
	for _, t := range targets {
		if !t.InScope {
			continue
		}
		name := strings.TrimSpace(t.TargetRaw)
		if len(name) > 60 {
			continue // skip long descriptive text assets
		}
		assetNames = append(assetNames, name)
		if len(assetNames) >= 5 {
			break
		}
	}

	base := fmt.Sprintf("%s on %s (%s, %d in-scope targets).",
		program.Handle, capitalizedPlatform(program.Platform), programType, inScopeCount)

	if len(assetNames) == 0 {
		return base
	}

	assets := strings.Join(assetNames, ", ")
	desc := fmt.Sprintf("%s In-scope: %s", base, assets)
	if len(assetNames) < inScopeCount {
		desc += ", ..."
	}
	desc += "."

	// Google truncates at ~155 chars; keep it under that
	if len(desc) > 155 {
		desc = desc[:152] + "..."
	}

	return desc
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
			{Label: "Wayback", URL: fmt.Sprintf("https://web.archive.org/web/*/%s", target), Description: "Wayback Machine archives"},
			{Label: "DNSdumpster", URL: fmt.Sprintf("https://dnsdumpster.com/?search=%s", url.QueryEscape(domain)), Description: "DNS recon and subdomain discovery"},
			{Label: "VirusTotal", URL: fmt.Sprintf("https://www.virustotal.com/gui/domain/%s", url.QueryEscape(domain)), Description: "VirusTotal domain analysis"},
			{Label: "AlienVault OTX", URL: fmt.Sprintf("https://otx.alienvault.com/indicator/domain/%s", url.QueryEscape(domain)), Description: "AlienVault OTX threat intelligence"},
			{Label: "GitHub", URL: fmt.Sprintf("https://github.com/search?q=%s&type=code", url.QueryEscape(domain)), Description: "GitHub code search"},
			{Label: "Fofa", URL: fmt.Sprintf("https://en.fofa.info/result?qbase64=%s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(`domain="%s"`, domain)))), Description: "Fofa asset search"},
		}
	case "cidr":
		return []QuickLink{
			{Label: "Shodan", URL: fmt.Sprintf("https://www.shodan.io/search?query=net:%s", url.QueryEscape(target)), Description: "Shodan network search"},
			{Label: "Censys", URL: fmt.Sprintf("https://search.censys.io/hosts?q=%s", url.QueryEscape(target)), Description: "Censys host search"},
			{Label: "BGP HE", URL: fmt.Sprintf("https://bgp.he.net/net/%s", target), Description: "Hurricane Electric BGP prefix info"},
		}
	case "android":
		pkg := extractPackageName(target)
		if pkg != "" {
			return []QuickLink{
				{Label: "Play Store", URL: fmt.Sprintf("https://play.google.com/store/apps/details?id=%s", url.QueryEscape(pkg)), Description: "Google Play Store listing"},
				{Label: "APKPure", URL: fmt.Sprintf("https://apkpure.com/search?q=%s", url.QueryEscape(pkg)), Description: "APKPure app download"},
				{Label: "APKMirror", URL: fmt.Sprintf("https://www.apkmirror.com/?post_type=app_release&searchtype=apk&s=%s", url.QueryEscape(pkg)), Description: "APKMirror app download"},
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
		Class("p-1.5 text-zinc-500 hover:text-cyan-400 transition-all duration-200 rounded-md hover:bg-zinc-800/50"),
		g.Attr("onclick", fmt.Sprintf("navigator.clipboard.writeText('%s').then(()=>{this.textContent='Copied!';setTimeout(()=>this.textContent='Copy',1500)})", escaped)),
		g.Attr("title", "Copy to clipboard"),
		g.Text("Copy"),
	)
}
