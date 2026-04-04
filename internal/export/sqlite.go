package export

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

// ToSQLite exports the given tables from a MySQL DevLake database to a SQLite file.
// It dynamically reads the schema from MySQL and recreates tables in SQLite.
func ToSQLite(mysqlDSN, sqlitePath string, tables []string) error {
	mysql, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		return fmt.Errorf("connecting to MySQL: %w", err)
	}
	defer mysql.Close()

	sqlite, err := sql.Open("sqlite3", sqlitePath)
	if err != nil {
		return fmt.Errorf("opening SQLite: %w", err)
	}
	defer sqlite.Close()

	// Enable WAL mode for better write performance.
	if _, err := sqlite.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("setting WAL mode: %w", err)
	}

	for _, table := range tables {
		fmt.Printf("  Exporting %s...", table)
		n, err := exportTable(mysql, sqlite, table)
		if err != nil {
			fmt.Println(" error")
			return fmt.Errorf("exporting table %s: %w", table, err)
		}
		fmt.Printf(" %d rows\n", n)
	}

	return nil
}

func exportTable(mysql, sqlite *sql.DB, table string) (int64, error) {
	// Get column info from MySQL.
	cols, err := getColumns(mysql, table)
	if err != nil {
		return 0, err
	}
	if len(cols) == 0 {
		return 0, fmt.Errorf("table %s not found or has no columns", table)
	}

	// Drop and recreate the table in SQLite.
	if _, err := sqlite.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %q", table)); err != nil {
		return 0, fmt.Errorf("dropping table: %w", err)
	}
	createSQL := buildCreateTable(table, cols)
	if _, err := sqlite.Exec(createSQL); err != nil {
		return 0, fmt.Errorf("creating table: %w", err)
	}

	// Read all rows from MySQL.
	colNames := make([]string, len(cols))
	for i, c := range cols {
		colNames[i] = c.name
	}
	quotedCols := make([]string, len(colNames))
	for i, name := range colNames {
		quotedCols[i] = fmt.Sprintf("`%s`", name)
	}

	rows, err := mysql.Query(fmt.Sprintf("SELECT %s FROM `%s`", strings.Join(quotedCols, ", "), table))
	if err != nil {
		return 0, fmt.Errorf("querying MySQL: %w", err)
	}
	defer rows.Close()

	// Prepare insert statement.
	placeholders := make([]string, len(colNames))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	insertSQL := fmt.Sprintf("INSERT INTO %q (%s) VALUES (%s)",
		table,
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)

	tx, err := sqlite.Begin()
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return 0, fmt.Errorf("preparing insert: %w", err)
	}
	defer stmt.Close()

	var count int64
	scanDest := make([]interface{}, len(colNames))
	scanPtrs := make([]interface{}, len(colNames))
	for i := range scanDest {
		scanPtrs[i] = &scanDest[i]
	}

	for rows.Next() {
		if err := rows.Scan(scanPtrs...); err != nil {
			return 0, fmt.Errorf("scanning row: %w", err)
		}
		// Convert []byte to string for SQLite compatibility.
		for i, v := range scanDest {
			if b, ok := v.([]byte); ok {
				scanDest[i] = string(b)
			}
		}
		if _, err := stmt.Exec(scanDest...); err != nil {
			return 0, fmt.Errorf("inserting row: %w", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating rows: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing transaction: %w", err)
	}
	return count, nil
}

type column struct {
	name    string
	colType string
	isNull  string
	colKey  string
}

func getColumns(db *sql.DB, table string) ([]column, error) {
	rows, err := db.Query(
		"SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COLUMN_KEY FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_NAME = ? AND TABLE_SCHEMA = DATABASE() ORDER BY ORDINAL_POSITION",
		table,
	)
	if err != nil {
		return nil, fmt.Errorf("querying column info: %w", err)
	}
	defer rows.Close()

	var cols []column
	for rows.Next() {
		var c column
		if err := rows.Scan(&c.name, &c.colType, &c.isNull, &c.colKey); err != nil {
			return nil, fmt.Errorf("scanning column info: %w", err)
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

func buildCreateTable(table string, cols []column) string {
	var b strings.Builder
	fmt.Fprintf(&b, "CREATE TABLE %q (\n", table)
	for i, c := range cols {
		sqliteType := mapType(c.colType)
		nullable := ""
		if c.isNull == "NO" {
			nullable = " NOT NULL"
		}
		pk := ""
		if c.colKey == "PRI" {
			pk = " PRIMARY KEY"
		}
		fmt.Fprintf(&b, "  `%s` %s%s%s", c.name, sqliteType, nullable, pk)
		if i < len(cols)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(")")
	return b.String()
}

func mapType(mysqlType string) string {
	switch strings.ToLower(mysqlType) {
	case "tinyint", "smallint", "mediumint", "int", "bigint", "bit", "boolean":
		return "INTEGER"
	case "float", "double", "decimal":
		return "REAL"
	case "blob", "mediumblob", "longblob", "binary", "varbinary":
		return "BLOB"
	default:
		return "TEXT"
	}
}
