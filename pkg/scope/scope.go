package scope

import (
	"fmt"
	"log"
	"os"
	"strings"
)

type ScopeElement struct {
	Target      string
	Description string
	Category    string
}

type ProgramData struct {
	Url        string
	InScope    []ScopeElement
	OutOfScope []ScopeElement
}

func PrintProgramScope(programScope ProgramData, outputFlags string, delimiter string, includeOOS bool) {
	printScope := func(scope []ScopeElement, prefix string) {
		for _, scopeElement := range scope {
			line := createLine(scopeElement, programScope.Url, outputFlags, delimiter)
			if len(line) > 0 {
				fmt.Println(prefix + line)
			}
		}
	}

	printScope(programScope.InScope, "")
	if includeOOS {
		printScope(programScope.OutOfScope, "[OOS] ")
	}
}

func createLine(scopeElement ScopeElement, url, outputFlags, delimiter string) string {
	var line strings.Builder
	for _, f := range outputFlags {
		switch f {
		case 't':
			if scopeElement.Target != "NO_IN_SCOPE_TABLE" {
				line.WriteString(scopeElement.Target + delimiter)
			} else {
				fmt.Fprint(os.Stderr, scopeElement.Target)
			}
		case 'd':
			line.WriteString(scopeElement.Description + delimiter)
		case 'c':
			line.WriteString(scopeElement.Category + delimiter)
		case 'u':
			line.WriteString(url + delimiter)
		default:
			log.Fatal("Invalid print flag")
		}
	}
	return strings.TrimSuffix(line.String(), delimiter)
}
