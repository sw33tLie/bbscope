package core

import (
	"net/http"

	g "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

func contactLink(icon, label, href string) g.Node {
	return A(Href(href), Target("_blank"), Rel("noopener noreferrer"),
		Class("flex items-center gap-3 px-4 py-3 bg-slate-800/30 border border-slate-700/50 rounded-xl hover:border-slate-600 hover:bg-slate-800/50 transition-all duration-200 group"),
		Div(Class("w-9 h-9 rounded-lg bg-cyan-500/10 border border-cyan-500/20 flex items-center justify-center flex-shrink-0 group-hover:bg-cyan-500/20 transition-colors duration-200"),
			g.Raw(icon),
		),
		Span(Class("text-sm text-slate-300 group-hover:text-cyan-400 transition-colors duration-200"), g.Text(label)),
	)
}

// ContactContent component for the /contact page
func ContactContent() g.Node {
	return Main(Class("container mx-auto mt-10 mb-20 px-4 max-w-2xl"),
		// Page header
		Div(Class("mb-10"),
			H1(Class("text-2xl md:text-3xl font-bold text-white mb-3"), g.Text("Contact")),
			P(Class("text-slate-400 text-lg"), g.Text("This website is created and maintained by sw33tLie.")),
		),

		// Get in Touch
		Section(Class("bg-slate-900/30 border border-slate-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8 mb-6"),
			H2(Class("text-lg font-semibold text-white mb-5"), g.Text("Get in Touch")),
			Div(Class("space-y-3"),
				contactLink(
					`<svg class="w-4 h-4 text-cyan-400" fill="currentColor" viewBox="0 0 24 24"><path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/></svg>`,
					"x.com/sw33tLie",
					"https://x.com/sw33tLie",
				),
				contactLink(
					`<svg class="w-4 h-4 text-cyan-400" fill="currentColor" viewBox="0 0 24 24"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>`,
					"github.com/sw33tLie",
					"https://github.com/sw33tLie",
				),
			),
		),

		// Contributing
		Section(Class("bg-slate-900/30 border border-slate-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8 mb-6"),
			H2(Class("text-lg font-semibold text-white mb-3"), g.Text("Found a Bug?")),
			P(Class("text-slate-400 leading-relaxed mb-4"), g.Text("Found an issue with bbscope or this website? Pull requests are welcome!")),
			A(Href("https://github.com/sw33tLie/bbscope"), Target("_blank"), Rel("noopener noreferrer"),
				Class("inline-flex items-center gap-2 px-4 py-2 bg-cyan-600 rounded-lg text-sm text-white font-medium hover:bg-cyan-500 transition-all duration-200 hover:shadow-md hover:shadow-cyan-500/20"),
				g.Text("Open an Issue"),
			),
		),

		// Collaboration
		Section(Class("bg-slate-900/30 border border-slate-800/50 rounded-2xl shadow-xl shadow-black/10 p-6 md:p-8"),
			H2(Class("text-lg font-semibold text-white mb-3"), g.Text("Collaboration & Bug Hunting")),
			P(Class("text-slate-400 leading-relaxed"), g.Text("Are you a fellow bug hunter stuck on a particularly tricky bug? Don't hesitate to reach out! I'm always open to collaboration and brainstorming. Feel free to send a DM!")),
		),
	)
}

// contactHandler handles requests for the /contact page.
func contactHandler(w http.ResponseWriter, r *http.Request) {
	PageLayout(
		"Contact Us - bbscope.com",
		"Get in touch with the maintainers of bbscope.com.",
		Navbar("/contact"),
		ContactContent(),
		FooterEl(),
		"",
		false,
	).Render(w)
}
