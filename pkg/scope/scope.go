package scope

import (
	"fmt"
	"log"
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
	var line string
	for _, f := range outputFlags {
		switch f {
		case 't':
			line += scopeElement.Target + delimiter
		case 'd':
			line += scopeElement.Description + delimiter
		case 'c':
			line += scopeElement.Category + delimiter
		case 'u':
			line += url + delimiter
		default:
			log.Fatal("Invalid print flag")
		}
	}
	return strings.TrimSuffix(line, delimiter)
}
