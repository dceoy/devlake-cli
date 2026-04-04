package cmd

import (
	"fmt"

	"github.com/dceoy/devlake-cli/internal/export"
	"github.com/spf13/cobra"
)

var (
	exportOutput string
	exportTables []string
)

// Default DevLake domain layer tables to export.
var defaultTables = []string{
	"boards",
	"cicd_deployments",
	"cicd_pipelines",
	"cicd_tasks",
	"commits",
	"issue_comments",
	"issue_labels",
	"issues",
	"pull_request_comments",
	"pull_request_commits",
	"pull_request_labels",
	"pull_requests",
	"refs",
	"repos",
	"users",
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export DevLake domain layer tables from MySQL to SQLite",
	RunE: func(cmd *cobra.Command, args []string) error {
		tables := exportTables
		if len(tables) == 0 {
			tables = defaultTables
		}
		fmt.Printf("Exporting %d tables to %s\n", len(tables), exportOutput)
		return export.ToSQLite(dbDSN, exportOutput, tables)
	},
}

func init() {
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "devlake.db", "output SQLite file path")
	exportCmd.Flags().StringSliceVarP(&exportTables, "tables", "t", nil, "tables to export (default: domain layer tables)")
	rootCmd.AddCommand(exportCmd)
}
