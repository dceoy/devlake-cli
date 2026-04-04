package export

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/linkedin/goavro/v2"
)

// ToIceberg exports the given tables from a MySQL DevLake database to Apache Iceberg format.
// Each table is written as an Iceberg v2 table with Parquet data files.
func ToIceberg(mysqlDSN, outputDir string, tables []string) error {
	mysqlDB, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		return fmt.Errorf("connecting to MySQL: %w", err)
	}
	defer mysqlDB.Close()

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	for _, table := range tables {
		fmt.Printf("  Exporting %s...", table)
		n, err := exportTableIceberg(mysqlDB, outputDir, table)
		if err != nil {
			fmt.Println(" error")
			return fmt.Errorf("exporting table %s: %w", table, err)
		}
		fmt.Printf(" %d rows\n", n)
	}
	return nil
}

func exportTableIceberg(db *sql.DB, baseDir, table string) (int64, error) {
	cols, err := getColumns(db, table)
	if err != nil {
		return 0, err
	}
	if len(cols) == 0 {
		return 0, fmt.Errorf("table %s not found or has no columns", table)
	}

	// Create Iceberg directory structure.
	tableDir := filepath.Join(baseDir, table)
	dataDir := filepath.Join(tableDir, "data")
	metadataDir := filepath.Join(tableDir, "metadata")
	for _, d := range []string{dataDir, metadataDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return 0, fmt.Errorf("creating directory %s: %w", d, err)
		}
	}

	// Build Arrow schema from MySQL columns.
	arrowSchema := buildArrowSchema(cols)

	// Read all rows from MySQL.
	colNames := make([]string, len(cols))
	for i, c := range cols {
		colNames[i] = c.name
	}
	quotedCols := make([]string, len(colNames))
	for i, name := range colNames {
		quotedCols[i] = fmt.Sprintf("`%s`", name)
	}

	rows, err := db.Query(fmt.Sprintf("SELECT %s FROM `%s`", strings.Join(quotedCols, ", "), table))
	if err != nil {
		return 0, fmt.Errorf("querying MySQL: %w", err)
	}
	defer rows.Close()

	// Build Arrow record batch from MySQL rows.
	alloc := memory.NewGoAllocator()
	builder := array.NewRecordBuilder(alloc, arrowSchema)
	defer builder.Release()

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
		for i, v := range scanDest {
			appendArrowValue(builder.Field(i), v)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating rows: %w", err)
	}

	rec := builder.NewRecord()
	defer rec.Release()

	// Write Parquet data file.
	dataFileName := "00000-0-data.parquet"
	dataFilePath := filepath.Join(dataDir, dataFileName)
	if err := writeParquetFile(dataFilePath, rec); err != nil {
		return 0, fmt.Errorf("writing parquet: %w", err)
	}

	fi, err := os.Stat(dataFilePath)
	if err != nil {
		return 0, fmt.Errorf("stating parquet file: %w", err)
	}

	// Generate Iceberg metadata files.
	tableUUID := uuid.New().String()
	snapshotID := time.Now().UnixMilli()
	nowMs := time.Now().UnixMilli()
	absTableDir, err := filepath.Abs(tableDir)
	if err != nil {
		return 0, fmt.Errorf("resolving table directory: %w", err)
	}
	absDataFilePath, err := filepath.Abs(dataFilePath)
	if err != nil {
		return 0, fmt.Errorf("resolving data file path: %w", err)
	}

	schemaJSON := buildIcebergSchemaJSON(cols)

	// Write manifest Avro file.
	manifestFileName := fmt.Sprintf("snap-%d-0-manifest.avro", snapshotID)
	manifestPath := filepath.Join(metadataDir, manifestFileName)
	if err := writeManifestFile(manifestPath, absDataFilePath, count, fi.Size(), schemaJSON, snapshotID); err != nil {
		return 0, fmt.Errorf("writing manifest: %w", err)
	}
	manifestFI, err := os.Stat(manifestPath)
	if err != nil {
		return 0, fmt.Errorf("stating manifest file: %w", err)
	}

	// Write manifest list Avro file.
	manifestListFileName := fmt.Sprintf("snap-%d-0-manifest-list.avro", snapshotID)
	manifestListPath := filepath.Join(metadataDir, manifestListFileName)
	absManifestPath, err := filepath.Abs(manifestPath)
	if err != nil {
		return 0, fmt.Errorf("resolving manifest path: %w", err)
	}
	if err := writeManifestListFile(manifestListPath, absManifestPath, manifestFI.Size(), count, snapshotID); err != nil {
		return 0, fmt.Errorf("writing manifest list: %w", err)
	}

	// Write metadata.json.
	absManifestListPath, err := filepath.Abs(manifestListPath)
	if err != nil {
		return 0, fmt.Errorf("resolving manifest list path: %w", err)
	}
	metadataJSON := buildMetadataJSON(tableUUID, absTableDir, absManifestListPath, cols, snapshotID, nowMs, count)
	metadataPath := filepath.Join(metadataDir, "v1.metadata.json")
	if err := os.WriteFile(metadataPath, metadataJSON, 0o644); err != nil {
		return 0, fmt.Errorf("writing metadata: %w", err)
	}

	// Write version-hint.text for catalog discovery.
	if err := os.WriteFile(filepath.Join(metadataDir, "version-hint.text"), []byte("1"), 0o644); err != nil {
		return 0, fmt.Errorf("writing version hint: %w", err)
	}

	return count, nil
}

func buildArrowSchema(cols []column) *arrow.Schema {
	fields := make([]arrow.Field, len(cols))
	for i, c := range cols {
		fields[i] = arrow.Field{
			Name:     c.name,
			Type:     mysqlToArrowType(c.colType),
			Nullable: c.isNull != "NO",
		}
	}
	return arrow.NewSchema(fields, nil)
}

func mysqlToArrowType(mysqlType string) arrow.DataType {
	switch strings.ToLower(mysqlType) {
	case "tinyint", "smallint", "mediumint", "int":
		return arrow.PrimitiveTypes.Int32
	case "bigint":
		return arrow.PrimitiveTypes.Int64
	case "boolean", "bit":
		return arrow.FixedWidthTypes.Boolean
	case "float":
		return arrow.PrimitiveTypes.Float32
	case "double":
		return arrow.PrimitiveTypes.Float64
	case "blob", "mediumblob", "longblob", "binary", "varbinary":
		return arrow.BinaryTypes.Binary
	default:
		return arrow.BinaryTypes.String
	}
}

func appendArrowValue(fb array.Builder, v interface{}) {
	if v == nil {
		fb.AppendNull()
		return
	}

	// The MySQL driver returns []byte for most non-numeric types.
	var val interface{} = v
	if b, ok := v.([]byte); ok {
		val = string(b)
	}

	switch b := fb.(type) {
	case *array.Int32Builder:
		switch n := val.(type) {
		case int64:
			b.Append(int32(n))
		default:
			b.AppendNull()
		}
	case *array.Int64Builder:
		switch n := val.(type) {
		case int64:
			b.Append(n)
		default:
			b.AppendNull()
		}
	case *array.Float32Builder:
		switch n := val.(type) {
		case float64:
			b.Append(float32(n))
		default:
			b.AppendNull()
		}
	case *array.Float64Builder:
		switch n := val.(type) {
		case float64:
			b.Append(n)
		default:
			b.AppendNull()
		}
	case *array.BooleanBuilder:
		switch n := val.(type) {
		case int64:
			b.Append(n != 0)
		case bool:
			b.Append(n)
		default:
			b.AppendNull()
		}
	case *array.BinaryBuilder:
		switch s := v.(type) {
		case []byte:
			b.Append(s)
		case string:
			b.Append([]byte(s))
		default:
			b.AppendNull()
		}
	case *array.StringBuilder:
		switch s := val.(type) {
		case string:
			b.Append(s)
		default:
			b.Append(fmt.Sprintf("%v", val))
		}
	default:
		fb.AppendNull()
	}
}

func writeParquetFile(path string, rec arrow.Record) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	writer, err := pqarrow.NewFileWriter(
		rec.Schema(), f,
		parquet.NewWriterProperties(),
		pqarrow.NewArrowWriterProperties(),
	)
	if err != nil {
		return fmt.Errorf("creating parquet writer: %w", err)
	}
	if err := writer.Write(rec); err != nil {
		return fmt.Errorf("writing record: %w", err)
	}
	return writer.Close()
}

// Avro schema for Iceberg v2 manifest entries.
const manifestEntryAvroSchema = `{
  "type": "record",
  "name": "manifest_entry",
  "fields": [
    {"name": "status", "type": "int", "field-id": 0},
    {"name": "snapshot_id", "type": ["null", "long"], "default": null, "field-id": 1},
    {"name": "sequence_number", "type": ["null", "long"], "default": null, "field-id": 3},
    {"name": "file_sequence_number", "type": ["null", "long"], "default": null, "field-id": 4},
    {"name": "data_file", "type": {
      "type": "record",
      "name": "r2",
      "fields": [
        {"name": "content", "type": "int", "field-id": 134},
        {"name": "file_path", "type": "string", "field-id": 100},
        {"name": "file_format", "type": "string", "field-id": 101},
        {"name": "partition", "type": {
          "type": "record",
          "name": "r102",
          "fields": []
        }, "field-id": 102},
        {"name": "record_count", "type": "long", "field-id": 103},
        {"name": "file_size_in_bytes", "type": "long", "field-id": 104}
      ]
    }, "field-id": 2}
  ]
}`

// Avro schema for Iceberg v2 manifest list entries.
const manifestListAvroSchema = `{
  "type": "record",
  "name": "manifest_file",
  "fields": [
    {"name": "manifest_path", "type": "string", "field-id": 500},
    {"name": "manifest_length", "type": "long", "field-id": 501},
    {"name": "partition_spec_id", "type": "int", "field-id": 502},
    {"name": "content", "type": "int", "field-id": 517},
    {"name": "sequence_number", "type": "long", "field-id": 515},
    {"name": "min_sequence_number", "type": "long", "field-id": 516},
    {"name": "added_snapshot_id", "type": "long", "field-id": 503},
    {"name": "added_data_files_count", "type": "int", "field-id": 504},
    {"name": "existing_data_files_count", "type": "int", "field-id": 505},
    {"name": "deleted_data_files_count", "type": "int", "field-id": 506},
    {"name": "added_rows_count", "type": "long", "field-id": 512},
    {"name": "existing_rows_count", "type": "long", "field-id": 513},
    {"name": "deleted_rows_count", "type": "long", "field-id": 514}
  ]
}`

func writeManifestFile(path, dataFilePath string, recordCount, fileSize int64, schemaJSON []byte, snapshotID int64) error {
	codec, err := goavro.NewCodec(manifestEntryAvroSchema)
	if err != nil {
		return fmt.Errorf("creating manifest codec: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	ocfWriter, err := goavro.NewOCFWriter(goavro.OCFConfig{
		W:     f,
		Codec: codec,
		MetaData: map[string][]byte{
			"format-version": []byte("2"),
			"content":        []byte("data"),
			"schema":         schemaJSON,
			"partition-spec":  []byte("[]"),
		},
	})
	if err != nil {
		return fmt.Errorf("creating OCF writer: %w", err)
	}

	entry := map[string]interface{}{
		"status":               int32(1), // ADDED
		"snapshot_id":          map[string]interface{}{"long": snapshotID},
		"sequence_number":      map[string]interface{}{"long": int64(1)},
		"file_sequence_number": map[string]interface{}{"long": int64(1)},
		"data_file": map[string]interface{}{
			"content":            int32(0), // DATA
			"file_path":          dataFilePath,
			"file_format":        "PARQUET",
			"partition":          map[string]interface{}{},
			"record_count":       recordCount,
			"file_size_in_bytes": fileSize,
		},
	}

	return ocfWriter.Append([]interface{}{entry})
}

func writeManifestListFile(path, manifestPath string, manifestLength, recordCount, snapshotID int64) error {
	codec, err := goavro.NewCodec(manifestListAvroSchema)
	if err != nil {
		return fmt.Errorf("creating manifest list codec: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	ocfWriter, err := goavro.NewOCFWriter(goavro.OCFConfig{
		W:     f,
		Codec: codec,
		MetaData: map[string][]byte{
			"format-version": []byte("2"),
		},
	})
	if err != nil {
		return fmt.Errorf("creating OCF writer: %w", err)
	}

	entry := map[string]interface{}{
		"manifest_path":             manifestPath,
		"manifest_length":           manifestLength,
		"partition_spec_id":         int32(0),
		"content":                   int32(0), // DATA
		"sequence_number":           int64(1),
		"min_sequence_number":       int64(1),
		"added_snapshot_id":         snapshotID,
		"added_data_files_count":    int32(1),
		"existing_data_files_count": int32(0),
		"deleted_data_files_count":  int32(0),
		"added_rows_count":          recordCount,
		"existing_rows_count":       int64(0),
		"deleted_rows_count":        int64(0),
	}

	return ocfWriter.Append([]interface{}{entry})
}

func buildIcebergSchemaJSON(cols []column) []byte {
	fields := make([]map[string]interface{}, len(cols))
	for i, c := range cols {
		fields[i] = map[string]interface{}{
			"id":       i + 1,
			"name":     c.name,
			"required": c.isNull == "NO",
			"type":     mysqlToIcebergType(c.colType),
		}
	}
	schema := map[string]interface{}{
		"type":      "struct",
		"schema-id": 0,
		"fields":    fields,
	}
	data, _ := json.Marshal(schema)
	return data
}

func mysqlToIcebergType(mysqlType string) string {
	switch strings.ToLower(mysqlType) {
	case "boolean", "bit":
		return "boolean"
	case "tinyint", "smallint", "mediumint", "int":
		return "int"
	case "bigint":
		return "long"
	case "float":
		return "float"
	case "double":
		return "double"
	case "decimal":
		return "decimal(38, 18)"
	case "date":
		return "date"
	case "datetime", "timestamp":
		return "timestamptz"
	case "blob", "mediumblob", "longblob", "binary", "varbinary":
		return "binary"
	default:
		return "string"
	}
}

func buildMetadataJSON(tableUUID, tableLocation, manifestListPath string, cols []column, snapshotID, nowMs, recordCount int64) []byte {
	fields := make([]map[string]interface{}, len(cols))
	for i, c := range cols {
		fields[i] = map[string]interface{}{
			"id":       i + 1,
			"name":     c.name,
			"required": c.isNull == "NO",
			"type":     mysqlToIcebergType(c.colType),
		}
	}

	metadata := map[string]interface{}{
		"format-version":       2,
		"table-uuid":           tableUUID,
		"location":             tableLocation,
		"last-sequence-number": 1,
		"last-updated-ms":      nowMs,
		"last-column-id":       len(cols),
		"current-schema-id":    0,
		"schemas": []interface{}{
			map[string]interface{}{
				"type":      "struct",
				"schema-id": 0,
				"fields":    fields,
			},
		},
		"default-spec-id": 0,
		"partition-specs": []interface{}{
			map[string]interface{}{
				"spec-id": 0,
				"fields":  []interface{}{},
			},
		},
		"last-partition-id":     999,
		"default-sort-order-id": 0,
		"sort-orders": []interface{}{
			map[string]interface{}{
				"order-id": 0,
				"fields":   []interface{}{},
			},
		},
		"current-snapshot-id": snapshotID,
		"snapshots": []interface{}{
			map[string]interface{}{
				"snapshot-id":     snapshotID,
				"timestamp-ms":    nowMs,
				"sequence-number": 1,
				"summary": map[string]interface{}{
					"operation":        "append",
					"added-data-files": "1",
					"added-records":    fmt.Sprintf("%d", recordCount),
					"total-data-files": "1",
					"total-records":    fmt.Sprintf("%d", recordCount),
				},
				"manifest-list": manifestListPath,
				"schema-id":     0,
			},
		},
		"snapshot-log": []interface{}{
			map[string]interface{}{
				"timestamp-ms": nowMs,
				"snapshot-id":  snapshotID,
			},
		},
		"metadata-log": []interface{}{},
		"properties":   map[string]interface{}{},
	}

	data, _ := json.MarshalIndent(metadata, "", "  ")
	return data
}
