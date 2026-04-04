package cmd

import (
	"fmt"
	"strings"

	"github.com/dceoy/devlake-cli/internal/export"
	"github.com/spf13/cobra"
)

var (
	exportOutput string
	exportFormat string
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
	Short: "Export DevLake domain layer tables",
	RunE: func(cmd *cobra.Command, args []string) error {
		tables := exportTables
		if len(tables) == 0 {
			tables = defaultTables
		}
		switch strings.ToLower(exportFormat) {
		case "sqlite":
			fmt.Printf("Exporting %d tables to %s (SQLite)\n", len(tables), exportOutput)
			return export.ToSQLite(dbDSN, exportOutput, tables)
		case "iceberg":
			fmt.Printf("Exporting %d tables to %s (Iceberg)\n", len(tables), exportOutput)
			return export.ToIceberg(dbDSN, exportOutput, tables)
		default:
			return fmt.Errorf("unsupported format: %s (supported: sqlite, iceberg)", exportFormat)
		}
	},
}

func init() {
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "devlake.db", "output file path or directory")
	exportCmd.Flags().StringVarP(&exportFormat, "format", "f", "sqlite", "output format (sqlite, iceberg)")
	exportCmd.Flags().StringSliceVarP(&exportTables, "tables", "t", nil, "tables to export (default: domain layer tables)")
	rootCmd.AddCommand(exportCmd)
}
