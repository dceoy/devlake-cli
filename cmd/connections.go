package cmd

import (
	"fmt"
	"text/tabwriter"
	"os"

	"github.com/dceoy/devlake-cli/internal/api"
	"github.com/spf13/cobra"
)

var connectionsCmd = &cobra.Command{
	Use:   "connections",
	Short: "Manage DevLake connections",
}

var connectionsListCmd = &cobra.Command{
	Use:   "list [plugin]",
	Short: "List connections for a plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient(apiURL)
		conns, err := client.ListConnections(args[0])
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME")
		for _, c := range conns {
			fmt.Fprintf(w, "%d\t%s\n", c.ID, c.Name)
		}
		return w.Flush()
	},
}

func init() {
	connectionsCmd.AddCommand(connectionsListCmd)
	rootCmd.AddCommand(connectionsCmd)
}
