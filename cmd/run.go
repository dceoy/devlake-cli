package cmd

import (
	"fmt"
	"time"

	"github.com/dceoy/devlake-cli/internal/api"
	"github.com/spf13/cobra"
)

var runPollInterval time.Duration

var runCmd = &cobra.Command{
	Use:   "run [blueprint-id]",
	Short: "Trigger a blueprint and wait for the pipeline to complete",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var id uint64
		if _, err := fmt.Sscanf(args[0], "%d", &id); err != nil {
			return fmt.Errorf("invalid blueprint ID: %s", args[0])
		}

		client := api.NewClient(apiURL)

		fmt.Printf("Triggering blueprint %d...\n", id)
		p, err := client.TriggerBlueprint(id)
		if err != nil {
			return err
		}
		fmt.Printf("Pipeline %d created. Waiting for completion...\n", p.ID)

		for {
			time.Sleep(runPollInterval)
			p, err = client.GetPipeline(p.ID)
			if err != nil {
				return err
			}
			switch p.Status {
			case "TASK_COMPLETED":
				fmt.Printf("Pipeline %d completed successfully.\n", p.ID)
				return nil
			case "TASK_FAILED":
				return fmt.Errorf("pipeline %d failed: %s", p.ID, p.Message)
			case "TASK_CANCELLED":
				return fmt.Errorf("pipeline %d was cancelled", p.ID)
			default:
				fmt.Printf("  Status: %s\n", p.Status)
			}
		}
	},
}

func init() {
	runCmd.Flags().DurationVar(&runPollInterval, "poll-interval", 5*time.Second, "polling interval for pipeline status")
	rootCmd.AddCommand(runCmd)
}
