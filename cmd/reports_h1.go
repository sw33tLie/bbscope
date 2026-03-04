package cmd

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/reports"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
)

var reportsH1Cmd = &cobra.Command{
	Use:   "h1",
	Short: "Download reports from HackerOne",
	RunE: func(cmd *cobra.Command, _ []string) error {
		user := viper.GetString("hackerone.username")
		token := viper.GetString("hackerone.token")
		if user == "" || token == "" {
			utils.Log.Error("hackerone requires a username and token")
			return nil
		}

		proxy, _ := rootCmd.Flags().GetString("proxy")
		if proxy != "" {
			whttp.SetupProxy(proxy)
		}

		outputDir, _ := cmd.Flags().GetString("output-dir")
		programs, _ := cmd.Flags().GetStringSlice("program")
		states, _ := cmd.Flags().GetStringSlice("state")
		severities, _ := cmd.Flags().GetStringSlice("severity")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		overwrite, _ := cmd.Flags().GetBool("overwrite")

		fetcher := reports.NewH1Fetcher(user, token)
		opts := reports.FetchOptions{
			Programs:   programs,
			States:     states,
			Severities: severities,
			DryRun:     dryRun,
			Overwrite:  overwrite,
			OutputDir:  outputDir,
		}

		return runReportsH1(cmd.Context(), fetcher, opts)
	},
}

func init() {
	reportsCmd.AddCommand(reportsH1Cmd)
	reportsH1Cmd.Flags().StringP("user", "u", "", "HackerOne username")
	reportsH1Cmd.Flags().StringP("token", "t", "", "HackerOne API token")
	viper.BindPFlag("hackerone.username", reportsH1Cmd.Flags().Lookup("user"))
	viper.BindPFlag("hackerone.token", reportsH1Cmd.Flags().Lookup("token"))
}

func runReportsH1(ctx context.Context, fetcher *reports.H1Fetcher, opts reports.FetchOptions) error {
	utils.Log.Info("Fetching report list from HackerOne...")

	summaries, err := fetcher.ListReports(ctx, opts)
	if err != nil {
		return fmt.Errorf("listing reports: %w", err)
	}

	utils.Log.Infof("Found %d reports", len(summaries))

	if len(summaries) == 0 {
		return nil
	}

	// Dry-run: print table and exit
	if opts.DryRun {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tPROGRAM\tSTATE\tSEVERITY\tCREATED\tTITLE")
		for _, s := range summaries {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				s.ID, s.ProgramHandle, s.Substate, s.SeverityRating, s.CreatedAt, s.Title)
		}
		w.Flush()
		return nil
	}

	// Download mode with worker pool
	var written, skipped, errored atomic.Int32
	total := len(summaries)

	workers := 10
	if total < workers {
		workers = total
	}

	jobs := make(chan int, total)
	for i := range summaries {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				s := summaries[i]
				utils.Log.Infof("[%d/%d] Fetching report %s: %s", i+1, total, s.ID, s.Title)

				report, err := fetcher.FetchReport(ctx, s.ID)
				if err != nil {
					utils.Log.Warnf("Error fetching report %s: %v", s.ID, err)
					errored.Add(1)
					continue
				}

				ok, err := reports.WriteReport(report, opts.OutputDir, opts.Overwrite)
				if err != nil {
					utils.Log.Warnf("Error writing report %s: %v", s.ID, err)
					errored.Add(1)
					continue
				}

				if ok {
					written.Add(1)
				} else {
					skipped.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	utils.Log.Infof("Done: %d written, %d skipped, %d errors", written.Load(), skipped.Load(), errored.Load())
	return nil
}
