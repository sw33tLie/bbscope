package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
	"github.com/sw33tLie/bbscope/v2/pkg/targets"
)

// getCmd represents the parent `db get` command.
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Extract specific scope types from the database based on format",
}

func getAndPrintTargets(targetType string, aggressive bool) error {
	dbURL, err := GetDBConnectionString()
	if err != nil {
		return err
	}

	db, err := storage.Open(dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	entries, err := db.ListEntries(context.Background(), storage.ListOptions{})
	if err != nil {
		return err
	}

	if aggressive {
		for i := range entries {
			entries[i].TargetNormalized = storage.AggressiveTransform(entries[i].TargetNormalized)
		}
	}

	var results []string
	switch targetType {
	case "urls":
		results = targets.CollectURLs(entries)
	case "ips":
		results = targets.CollectIPs(entries)
	case "cidrs":
		results = targets.CollectCIDRs(entries)
	case "domains":
		results = targets.CollectDomains(entries)
	}

	for _, r := range results {
		fmt.Println(r)
	}

	return nil
}

func init() {
	dbCmd.AddCommand(getCmd)
}
