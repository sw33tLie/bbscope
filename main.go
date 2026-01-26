package main

import (
	"fmt"

	"github.com/sw33tLie/bbscope/cmd"
)

func main() {
	fmt.Println("\033[33m[WARNING] You are using the legacy version of bbscope (v1).\033[0m")
	fmt.Println("\033[33mDevelopment has moved to the 'main' branch (v2).\033[0m")
	fmt.Println("\033[33mPlease update: git fetch && git checkout main\033[0m")
	fmt.Println("")
	cmd.Execute()
}
