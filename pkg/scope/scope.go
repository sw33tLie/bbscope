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

func PrintProgramScope(programScope ProgramData, outputFlags string, delimiter string) {
	lines := ""
	for _, scopeElement := range programScope.InScope {
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
				line += programScope.Url + delimiter
			default:
				log.Fatal("Invalid print flag")
			}
		}
		line = strings.TrimSuffix(line, delimiter)
		if len(line) > 0 {
			lines += line + "\n"
		}
	}

	lines = strings.TrimSuffix(lines, "\n")

	if len(lines) > 0 {
		fmt.Println(lines)
	}
}
