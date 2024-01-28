package main

import (
	b64 "encoding/base64"
	"flag"
	"fmt"

	"github.com/sw33tLie/bbscope/pkg/platforms/hackerone"
)

func main() {
	// Usage: go run *.go -token "your_h1_token" -username "your_h1_username"

	userFlag := flag.String("username", "", "HackerOne username")
	tokenFlag := flag.String("token", "", "HackerOne API Token")

	// Parse the command-line flags
	flag.Parse()

	if *userFlag == "" {
		fmt.Println("Username is required. Please provide the username using -username flag.")
		return
	}

	if *tokenFlag == "" {
		fmt.Println("Token is required. Please provide the token using -token flag.")
		return
	}

	// All platforms are supported, syntax is similar
	scope := hackerone.GetAllProgramsScope(b64.StdEncoding.EncodeToString([]byte(*userFlag+":"+*tokenFlag)), true, true, false, "all", true, 2, false, "", "", true)

	for _, s := range scope {
		for _, elem := range s.InScope {
			fmt.Println(elem.Target, elem.Category)
		}
	}
}
