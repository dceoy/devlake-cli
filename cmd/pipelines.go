package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dceoy/devlake-cli/internal/api"
	"github.com/spf13/cobra"
)

var pipelinesCmd = &cobra.Command{
	Use:   "pipelines",
	Short: "Manage DevLake pipelines",
}

var pipelinesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all pipelines",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient(apiURL)
		pl, err := client.ListPipelines()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSTATUS\tCREATED")
		for _, p := range pl.Pipelines {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", p.ID, p.Name, p.Status, p.CreatedAt)
		}
		return w.Flush()
	},
}

var pipelinesStatusCmd = &cobra.Command{
	Use:   "status [id]",
	Short: "Show status of a pipeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var id uint64
		if _, err := fmt.Sscanf(args[0], "%d", &id); err != nil {
			return fmt.Errorf("invalid pipeline ID: %s", args[0])
		}
		client := api.NewClient(apiURL)
		p, err := client.GetPipeline(id)
		if err != nil {
			return err
		}
		fmt.Printf("Pipeline %d\n", p.ID)
		fmt.Printf("  Name:    %s\n", p.Name)
		fmt.Printf("  Status:  %s\n", p.Status)
		if p.Message != "" {
			fmt.Printf("  Message: %s\n", p.Message)
		}
		fmt.Printf("  Created: %s\n", p.CreatedAt)
		fmt.Printf("  Updated: %s\n", p.UpdatedAt)
		return nil
	},
}

func init() {
	pipelinesCmd.AddCommand(pipelinesListCmd, pipelinesStatusCmd)
	rootCmd.AddCommand(pipelinesCmd)
}
