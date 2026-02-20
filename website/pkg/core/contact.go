package core

import (
	"net/http"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/parser"
	g "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// --- Contact Page Content ---

const contactMarkdownContent = `
# Contact Us

This website, bbscope.com, is created and maintained by **sw33tLie**.

## Get in Touch

*   **X Profile:** [x.com/sw33tLie](https://x.com/sw33tLie)
*   **GitHub:** [github.com/sw33tLie](https://github.com/sw33tLie)

Found a bug in [bbscope](https://github.com/sw33tLie/bbscope)? PRs are welcome!

## Collaboration & Bug Hunting

Are you a fellow bug hunter stuck on a particularly tricky bug? Don't hesitate to reach out! I'm always open to collaboration and brainstorming.

Feel free to send a DM!
`

// ContactContent component for the /contact page
func ContactContent() g.Node {
	// Configure markdown parser extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs
	p := parser.NewWithExtensions(extensions)

	htmlOutput := markdown.ToHTML([]byte(contactMarkdownContent), p, nil)

	return Main(Class("container mx-auto mt-8 mb-16 p-4"),
		Section(Class("md:bg-white md:rounded-lg md:shadow-xl md:p-8 lg:p-12 prose lg:prose-xl max-w-4xl mx-auto"), // MODIFIED classes: e.g., changed prose-xl to prose and lg:prose-2xl to lg:prose-xl
			g.Raw(string(htmlOutput)),
		),
	)
}

// contactHandler handles requests for the /contact page.
func contactHandler(w http.ResponseWriter, r *http.Request) {
	PageLayout(
		"Contact Us - bbscope.com",
		"Get in touch with the maintainers of bbscope.com.",
		Navbar(),
		ContactContent(),
		FooterEl(),
		"",
		false, // Not a noindex page
	).Render(w)
}
