package cmd

import (
	"github.com/spf13/cobra"
)

var (
	apiURL string
	dbDSN  string
)

var rootCmd = &cobra.Command{
	Use:   "devlake-cli",
	Short: "CLI for Apache DevLake",
	Long:  "A thin CLI to interact with Apache DevLake APIs and export data to SQLite or Apache Iceberg.",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "http://localhost:8080", "DevLake API base URL")
	rootCmd.PersistentFlags().StringVar(&dbDSN, "db-dsn", "merico:merico@tcp(localhost:3306)/lake", "DevLake MySQL DSN")
}

func Execute() error {
	return rootCmd.Execute()
}
