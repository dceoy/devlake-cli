package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dceoy/devlake-cli/internal/api"
	"github.com/spf13/cobra"
)

var blueprintsCmd = &cobra.Command{
	Use:   "blueprints",
	Short: "Manage DevLake blueprints",
}

var blueprintsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all blueprints",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient(apiURL)
		bps, err := client.ListBlueprints()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tMODE\tENABLED")
		for _, bp := range bps {
			fmt.Fprintf(w, "%d\t%s\t%s\t%v\n", bp.ID, bp.Name, bp.Mode, bp.Enable)
		}
		return w.Flush()
	},
}

var blueprintsTriggerCmd = &cobra.Command{
	Use:   "trigger [id]",
	Short: "Trigger a blueprint to start a pipeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var id uint64
		if _, err := fmt.Sscanf(args[0], "%d", &id); err != nil {
			return fmt.Errorf("invalid blueprint ID: %s", args[0])
		}
		client := api.NewClient(apiURL)
		p, err := client.TriggerBlueprint(id)
		if err != nil {
			return err
		}
		fmt.Printf("Pipeline %d created (status: %s)\n", p.ID, p.Status)
		return nil
	},
}

func init() {
	blueprintsCmd.AddCommand(blueprintsListCmd, blueprintsTriggerCmd)
	rootCmd.AddCommand(blueprintsCmd)
}
