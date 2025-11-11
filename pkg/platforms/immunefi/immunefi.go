package immunefi

import "strings"

const (
	PLATFORM_URL = "https://immunefi.com"
)

func getCategories(input string) []string {
	categories := map[string][]string{
		"web":       {"websites_and_applications"},
		"contracts": {"smart_contract"},
		"all":       {"websites_and_applications", "smart_contract"},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]
	if !ok {
		// Default to all if category is invalid
		return categories["all"]
	}
	return selectedCategory
}
