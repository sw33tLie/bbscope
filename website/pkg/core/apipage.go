package core

import (
	"net/http"

	g "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

func apiPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api" {
		http.NotFound(w, r)
		return
	}
	PageLayout(
		"API - bbscope.com",
		"Public API documentation for bbscope.com. Access bug bounty scope data programmatically.",
		Navbar("/api"),
		APIPageContent(),
		FooterEl(),
		"/api",
		false,
	).Render(w)
}

func APIPageContent() g.Node {
	return Main(Class("container mx-auto mt-10 mb-20 px-4 max-w-4xl"),
		Div(Class("mb-10"),
			H1(Class("text-2xl md:text-3xl font-bold text-white mb-3"), g.Text("API")),
			P(Class("text-zinc-400 text-lg"), g.Text("Access bug bounty scope data programmatically. All endpoints are public and require no authentication.")),
		),

		Section(Class("bg-zinc-900/30 border border-zinc-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8 mb-6"),
			H2(Class("text-lg font-semibold text-white mb-2"), g.Text("Try It")),
			P(Class("text-zinc-400 mb-5 text-sm"), g.Text("Build a request, preview the results, or download them as a file.")),

			Div(Class("grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mb-4"),
				Div(
					Label(Class("block text-sm font-medium text-zinc-400 mb-1.5"), g.Text("Endpoint")),
					Select(
						ID("api-try-endpoint"),
						Class("w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-zinc-200 text-sm focus:ring-2 focus:ring-cyan-500 focus:border-cyan-500"),
						Option(Value("wildcards"), g.Text("Wildcards")),
						Option(Value("domains"), g.Text("Domains")),
						Option(Value("urls"), g.Text("URLs")),
						Option(Value("ips"), g.Text("IPs")),
						Option(Value("cidrs"), g.Text("CIDRs")),
					),
				),
				Div(
					Label(Class("block text-sm font-medium text-zinc-400 mb-1.5"), g.Text("Scope")),
					Select(
						ID("api-try-scope"),
						Class("w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-zinc-200 text-sm focus:ring-2 focus:ring-cyan-500 focus:border-cyan-500"),
						Option(Value("in"), g.Text("In Scope (default)")),
						Option(Value("out"), g.Text("Out of Scope")),
						Option(Value("all"), g.Text("All")),
					),
				),
				Div(
					Label(Class("block text-sm font-medium text-zinc-400 mb-1.5"), g.Text("Platform")),
					Select(
						ID("api-try-platform"),
						Class("w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-zinc-200 text-sm focus:ring-2 focus:ring-cyan-500 focus:border-cyan-500"),
						Option(Value(""), g.Text("All Platforms")),
						Option(Value("h1"), g.Text("HackerOne")),
						Option(Value("bc"), g.Text("Bugcrowd")),
						Option(Value("it"), g.Text("Intigriti")),
						Option(Value("ywh"), g.Text("YesWeHack")),
					),
				),
				Div(
					Label(Class("block text-sm font-medium text-zinc-400 mb-1.5"), g.Text("Program Type")),
					Select(
						ID("api-try-type"),
						Class("w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-zinc-200 text-sm focus:ring-2 focus:ring-cyan-500 focus:border-cyan-500"),
						Option(Value(""), g.Text("All")),
						Option(Value("bbp"), g.Text("Bug Bounty (BBP)")),
						Option(Value("vdp"), g.Text("VDP")),
					),
				),
				Div(
					Label(Class("block text-sm font-medium text-zinc-400 mb-1.5"), g.Text("Data Source")),
					Select(
						ID("api-try-raw"),
						Class("w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-zinc-200 text-sm focus:ring-2 focus:ring-cyan-500 focus:border-cyan-500"),
						Option(Value(""), g.Text("AI Enhanced (default)")),
						Option(Value("true"), g.Text("Raw")),
					),
				),
			),

			Div(Class("bg-zinc-950 rounded-lg p-3 font-mono text-sm text-cyan-400 mb-4 overflow-x-auto"),
				Span(ID("api-try-url"), g.Text("/api/v1/targets/wildcards")),
			),

			Div(Class("flex flex-wrap gap-3"),
				Button(
					ID("api-try-preview"),
					Type("button"),
					Class("px-5 py-2.5 bg-cyan-600 text-white font-medium rounded-lg hover:bg-cyan-500 transition-all duration-200 hover:shadow-md hover:shadow-cyan-500/20 text-sm"),
					g.Text("Preview"),
				),
				Button(
					ID("api-try-download"),
					Type("button"),
					Class("px-5 py-2.5 bg-zinc-700 text-zinc-200 font-medium rounded-lg hover:bg-zinc-600 transition-all duration-200 text-sm"),
					g.Text("Download .txt"),
				),
				Button(
					ID("api-try-copy"),
					Type("button"),
					Class("px-5 py-2.5 bg-zinc-700 text-zinc-200 font-medium rounded-lg hover:bg-zinc-600 transition-all duration-200 text-sm"),
					g.Text("Copy curl command"),
				),
			),

			Pre(
				ID("api-try-output"),
				Class("hidden mt-4 bg-zinc-950 rounded-lg p-4 font-mono text-xs text-zinc-300 overflow-x-auto max-h-96 overflow-y-auto border border-zinc-800"),
			),
		),

		Section(Class("bg-zinc-900/30 border border-zinc-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8 mb-6"),
			H2(Class("text-lg font-semibold text-white mb-4"), g.Text("Usage Examples")),
			Div(Class("bg-zinc-950 rounded-lg p-4 font-mono text-sm text-cyan-400 overflow-x-auto"),
				g.Raw(`<span class="text-zinc-500"># Get all in-scope wildcard domains</span><br>curl -s https://bbscope.com/api/v1/targets/wildcards<br><br><span class="text-zinc-500"># Pipe directly into your tools</span><br>curl -s https://bbscope.com/api/v1/targets/wildcards | subfinder -silent<br><br><span class="text-zinc-500"># Filter by platform and get JSON</span><br>curl -s "https://bbscope.com/api/v1/targets/domains?platform=h1&amp;format=json"<br><br><span class="text-zinc-500"># Raw data without AI enhancements</span><br>curl -s "https://bbscope.com/api/v1/targets/wildcards?raw=true"<br><br><span class="text-zinc-500"># Get scope updates (since: today, yesterday, 7d, 30d, 90d, 1y, or YYYY-MM-DD)</span><br>curl -s "https://bbscope.com/api/v1/updates?since=7d"<br><br><span class="text-zinc-500"># Filter updates by platform and date range</span><br>curl -s "https://bbscope.com/api/v1/updates?since=2025-01-01&amp;until=2025-01-31&amp;platform=h1"<br><br><span class="text-zinc-500"># Search updates and paginate</span><br>curl -s "https://bbscope.com/api/v1/updates?search=example.com&amp;per_page=50&amp;page=2"`),
			),
		),

		Section(Class("bg-zinc-900/30 border border-zinc-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8 mb-6"),
			H2(Class("text-lg font-semibold text-white mb-5"), g.Text("Endpoint Reference")),

			H3(Class("text-sm font-semibold text-zinc-300 uppercase tracking-wider mb-4"), g.Text("Targets")),
			P(Class("text-zinc-400 mb-4 text-sm"), g.Text("Returns newline-delimited text by default. Add ?format=json for a JSON array.")),

			apiTargetEndpointCard("wildcards", "Wildcard root domains, useful for subdomain enumeration."),
			apiTargetEndpointCard("domains", "Domains (non-URL, non-wildcard targets)."),
			apiTargetEndpointCard("urls", "URL targets (http:// or https://)."),
			apiTargetEndpointCard("ips", "IP addresses (extracted from IPs and URLs)."),
			apiTargetEndpointCard("cidrs", "CIDR ranges and IP ranges."),

			Div(Class("bg-zinc-800/50 border border-zinc-700/50 rounded-lg p-4 mt-5 mb-6"),
				H4(Class("text-sm font-semibold text-zinc-300 mb-2"), g.Text("Query Parameters")),
				Div(Class("space-y-1.5"),
					apiParamRow("scope", "string", "in (default), out, or all"),
					apiParamRow("platform", "string", "h1, bc, it, or ywh"),
					apiParamRow("type", "string", "bbp or vdp"),
					apiParamRow("raw", "boolean", "true to skip AI enhancements and use raw platform data"),
					apiParamRow("format", "string", "json for JSON array output"),
				),
			),

			H3(Class("text-sm font-semibold text-zinc-300 uppercase tracking-wider mb-4"), g.Text("Programs")),

			apiEndpointCard(
				"GET", "/api/v1/programs",
				"Returns the full list of bug bounty programs with scope data as JSON.",
				[]apiParam{
					{"raw", "boolean", "Set to true for raw target data without AI enhancements"},
				},
			),

			apiEndpointCard(
				"GET", "/api/v1/programs/{platform}/{handle}",
				"Returns details for a single program including in-scope and out-of-scope targets.",
				[]apiParam{
					{"raw", "boolean", "Set to true for raw target data without AI enhancements"},
				},
			),

			H3(Class("text-sm font-semibold text-zinc-300 uppercase tracking-wider mb-4 mt-6"), g.Text("Updates")),

			apiEndpointCard(
				"GET", "/api/v1/updates",
				"Returns paginated scope changes (assets and programs added/removed) with time range filtering.",
				[]apiParam{
					{"page", "integer", "Page number (default: 1)"},
					{"per_page", "integer", "Results per page, max 250 (default: 25)"},
					{"platform", "string", "h1, bc, it, or ywh"},
					{"search", "string", "Search in targets, handles, categories"},
					{"since", "string", "Start of time range: today, yesterday, 7d, 30d, 90d, 1y, or YYYY-MM-DD"},
					{"until", "string", "End of time range: YYYY-MM-DD"},
				},
			),
		),

		Script(g.Raw(apiPageScript)),
	)
}

type apiParam struct {
	name, typ, desc string
}

func apiParamRow(name, typ, desc string) g.Node {
	return Div(Class("flex items-baseline gap-2 text-sm"),
		Code(Class("text-cyan-400 bg-zinc-900 px-1.5 py-0.5 rounded text-xs font-mono"), g.Text(name)),
		Span(Class("text-zinc-500 text-xs"), g.Text(typ)),
		Span(Class("text-zinc-400"), g.Text("— "+desc)),
	)
}

func apiEndpointCard(method, path, description string, params []apiParam) g.Node {
	paramNodes := []g.Node{}
	for _, p := range params {
		paramNodes = append(paramNodes, apiParamRow(p.name, p.typ, p.desc))
	}

	children := []g.Node{
		Div(Class("flex items-center gap-3 mb-2 min-w-0"),
			Span(Class("px-2 py-0.5 bg-emerald-900/50 text-emerald-400 border border-emerald-800 rounded text-xs font-semibold font-mono flex-shrink-0"), g.Text(method)),
			Code(Class("text-sm text-zinc-200 font-mono break-all"), g.Text(path)),
		),
		P(Class("text-sm text-zinc-400 mb-3"), g.Text(description)),
	}

	if len(paramNodes) > 0 {
		children = append(children,
			Div(Class("space-y-1.5"), g.Group(paramNodes)),
		)
	}

	return Div(Class("border-b border-zinc-800/50 pb-5 mb-5 last:border-0 last:pb-0 last:mb-0"),
		g.Group(children),
	)
}

func apiTargetEndpointCard(targetType, description string) g.Node {
	return Div(Class("border-b border-zinc-800/50 pb-4 mb-4 last:border-0 last:pb-0 last:mb-0"),
		Div(Class("flex items-center gap-3 mb-1.5 min-w-0"),
			Span(Class("px-2 py-0.5 bg-emerald-900/50 text-emerald-400 border border-emerald-800 rounded text-xs font-semibold font-mono flex-shrink-0"), g.Text("GET")),
			Code(Class("text-sm text-zinc-200 font-mono break-all"), g.Text("/api/v1/targets/"+targetType)),
		),
		P(Class("text-sm text-zinc-400"), g.Text(description)),
	)
}

const apiPageScript = `
(function() {
	const endpoint = document.getElementById('api-try-endpoint');
	const scope = document.getElementById('api-try-scope');
	const platform = document.getElementById('api-try-platform');
	const ptype = document.getElementById('api-try-type');
	const rawMode = document.getElementById('api-try-raw');
	const urlPreview = document.getElementById('api-try-url');
	const downloadBtn = document.getElementById('api-try-download');
	const previewBtn = document.getElementById('api-try-preview');
	const copyBtn = document.getElementById('api-try-copy');
	const output = document.getElementById('api-try-output');

	function buildURL() {
		let url = '/api/v1/targets/' + endpoint.value;
		const params = [];
		if (scope.value !== 'in') params.push('scope=' + scope.value);
		if (platform.value) params.push('platform=' + platform.value);
		if (ptype.value) params.push('type=' + ptype.value);
		if (rawMode.value) params.push('raw=true');
		if (params.length > 0) url += '?' + params.join('&');
		return url;
	}

	function updatePreview() {
		urlPreview.textContent = buildURL();
	}

	[endpoint, scope, platform, ptype, rawMode].forEach(function(el) {
		el.addEventListener('change', updatePreview);
	});

	downloadBtn.addEventListener('click', function() {
		const url = buildURL();
		const a = document.createElement('a');
		a.href = url;
		a.download = endpoint.value + '.txt';
		document.body.appendChild(a);
		a.click();
		document.body.removeChild(a);
	});

	previewBtn.addEventListener('click', function() {
		const url = buildURL();
		output.classList.remove('hidden');
		output.textContent = 'Loading...';
		fetch(url)
			.then(function(r) { return r.text(); })
			.then(function(text) {
				const lines = text.split('\n').slice(0, 50);
				output.textContent = lines.join('\n');
				if (text.split('\n').length > 50) {
					output.textContent += '\n\n... (truncated, download for full results)';
				}
			})
			.catch(function(err) {
				output.textContent = 'Error: ' + err.message;
			});
	});

	copyBtn.addEventListener('click', function() {
		const url = window.location.origin + buildURL();
		const cmd = 'curl -s "' + url + '"';
		navigator.clipboard.writeText(cmd).then(function() {
			const orig = copyBtn.textContent;
			copyBtn.textContent = 'Copied!';
			setTimeout(function() { copyBtn.textContent = orig; }, 1500);
		});
	});
})();
`
