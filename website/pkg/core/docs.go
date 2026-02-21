package core

import (
	"net/http"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/parser"
	g "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// --- Documentation Content ---

const docsMarkdownContent = `
# Documentation

Welcome to the bbscope.com documentation!

## Introduction

This platform helps you track bug bounty program scopes.

## Features

*   **View Scope Data**: Browse through aggregated scope information.
*   **Search**: Quickly find specific targets, categories, or programs.
*   **Track Changes**: (Coming Soon) Get notified about scope updates.

## How to Use

1.  Navigate to the [Scope Data](/scope) page.
2.  Use the search bar to filter results.
3.  Click on program URLs to visit the original program pages.

## API

(Coming Soon) We plan to provide an API for programmatic access to scope data.

## Contributing

bbscope is an open-source project. Contributions are welcome!
Please check out our [GitHub repository](https://github.com/sw33tLie/bbscope).
`

// DocsContent component for the /docs page
func DocsContent() g.Node {
	// Configure markdown parser extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs
	p := parser.NewWithExtensions(extensions)

	htmlOutput := markdown.ToHTML([]byte(docsMarkdownContent), p, nil)

	return Main(Class("container mx-auto mt-8 mb-16 p-4"),
		Section(Class("bg-slate-900/50 border border-slate-800 rounded-lg shadow-xl p-6 md:p-8 lg:p-12 prose lg:prose-xl prose-invert max-w-4xl mx-auto"),
			g.Raw(string(htmlOutput)), // Use g.Raw to render the HTML string
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
		Navbar(),
		DocsContent(),
		FooterEl(),
		"",
		false, // Not a noindex page
	).Render(w)
}
