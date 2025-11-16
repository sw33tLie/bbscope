package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// getCmd represents the parent `db get` command.
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Extract specific scope types from the database based on format",
}

func getAndPrintTargets(targetType string, aggressive bool) error {
	dbPath, _ := getCmd.PersistentFlags().GetString("dbpath")
	if dbPath == "" {
		dbPath = "bbscope.sqlite"
	}
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("database not found: %s", dbPath)
		}
		return err
	}

	db, err := storage.Open(dbPath, storage.DefaultDBTimeout)
	if err != nil {
		return err
	}
	defer db.Close()

	entries, err := db.ListEntries(context.Background(), storage.ListOptions{})
	if err != nil {
		return err
	}

	for _, e := range entries {
		target := e.TargetNormalized
		if aggressive {
			target = storage.AggressiveTransform(target)
		}

		switch targetType {
		case "urls":
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
				fmt.Println(target)
			}
		case "ips":
			if utils.IsIP(target) {
				fmt.Println(target)
				continue
			}
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
				if u, err := url.Parse(target); err == nil {
					host := strings.Trim(u.Hostname(), "[]")
					if utils.IsIP(host) {
						fmt.Println(host)
					}
				}
			}
		case "cidrs":
			if utils.IsCIDR(target) || utils.IsIPRange(target) {
				fmt.Println(target)
			}
		case "wildcards":
			if strings.HasPrefix(target, "*.") {
				fmt.Println(target)
			}
		case "domains":
			if strings.Contains(target, ".") && !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
				fmt.Println(target)
			}
		}
	}

	return nil
}

func init() {
	dbCmd.AddCommand(getCmd)
}
