#!/usr/bin/env bash

# Script to clean up vendor directory and fix dependencies

set -e

echo "===========> Cleaning vendor directory"
rm -rf vendor
go mod vendor

echo "===========> Patching sql-migrate dependency"
mkdir -p vendor/github.com/rubenv/sql-migrate
cp -r $(go env GOPATH)/pkg/mod/github.com/rubenv/sql-migrate@v1.5.2/* vendor/github.com/rubenv/sql-migrate/
chmod -R +w vendor/github.com/rubenv/sql-migrate/

# Create a patched version of migrate.go
cat > vendor/github.com/rubenv/sql-migrate/migrate.go << 'EOF'
package migrate

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-gorp/gorp/v3"
)

type MigrationDirection int

const (
	Up MigrationDirection = iota
	Down
)

// MigrationSet provides database migrations management
type MigrationSet struct {
	// TableName name of the table used to store migration info.
	TableName string
	// SchemaName schema that the migration table be referenced.
	SchemaName string
}

var migSet = MigrationSet{}

var DialectSQLite = "sqlite3"
var DialectMySQL = "mysql"
var DialectPostgres = "postgres"
var DialectMSSQL = "mssql"
var DialectOracle = "oci8"
var DialectSnowflake = "snowflake"

var migrationTable = "gorp_migrations"
var migrationSchema = ""
var migrationTableWithSchema = ""

// This is the query used to create/check the migrations table on SQLite.
// In order to support both PostgreSQL and SQLite for automated tests,
// it's a bit more verbose than it needs to be. SQLite doesn't create
// schemas and doesn't have serial types.
var sqliteCreateMigrationTableQuery = `
CREATE TABLE IF NOT EXISTS ` + QuoteIdentifier(migrationTable) + ` (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	` + QuoteIdentifier("table_name") + ` TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS ` + QuoteIdentifier(migrationTable+"_id_seq") + ` (
	id INTEGER PRIMARY KEY AUTOINCREMENT
);
`

// SetTable replaces the default migration table name with the given set.
func SetTable(name string) {
	migrationTable = name
	migrationTableWithSchema = ""
}

// SetSchema replaces the default migration schema with the given set.
func SetSchema(name string) {
	migrationSchema = name
	migrationTableWithSchema = ""
}

func GetMigrationTableName() string {
	return migrationTable
}

func GetSchemaTableName() string {
	return migrationSchema
}

func hasMigrationTable(db *sql.DB, dialect string) (bool, error) {
	var exists bool

	if dialect == DialectSQLite {
		// SQLite does not support `information_schema`
		// Instead, we check if the table exists.
		countSql := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = '%s');", migrationTable)
		row := db.QueryRow(countSql)
		err := row.Scan(&exists)
		if err != nil {
			return false, err
		}
		return exists, nil
	}

	countSql := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = '%s'", migrationTable)
	if migrationSchema != "" {
		countSql = fmt.Sprintf("%s AND table_schema = '%s'", countSql, migrationSchema)
	} else if dialect == DialectPostgres {
		countSql = fmt.Sprintf("%s AND table_schema = (SELECT current_schema())", countSql)
	}
	countSql = fmt.Sprintf("%s);", countSql)

	row := db.QueryRow(countSql)
	err := row.Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// Initialize creates the migration table
// Also maintains backwards compatibility
func Initialize(db *sql.DB, dialect string) error {
	if dialect == "" {
		return errors.New("no dialect specified")
	}

	var err error
	migrationTableWithSchema, err = quotedMigrationTableName(dialect)
	if err != nil {
		return err
	}

	var exists bool
	exists, err = hasMigrationTable(db, dialect)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	// Try legacy "gorp_migrations" table
	migrationTable = "gorp_migrations"
	migrationTableWithSchema, err = quotedMigrationTableName(dialect)
	if err != nil {
		return err
	}

	exists, err = hasMigrationTable(db, dialect)
	if err != nil {
		return err
	}

	// Use legacy table, and there's nothing left for us to do
	if exists {
		return nil
	}

	// No migration table found, we need to create it
	migrationTable = migSet.TableName
	migrationSchema = migSet.SchemaName
	migrationTableWithSchema, err = quotedMigrationTableName(dialect)
	if err != nil {
		return err
	}

	sqlStmt := ""
	if dialect == DialectSQLite {
		sqlStmt = sqliteCreateMigrationTableQuery
	} else {
		var id string
		if dialect == DialectPostgres {
			id = "serial"
		} else if dialect == DialectSnowflake {
			id = "integer AUTOINCREMENT"
		} else {
			id = "int(11) NOT NULL AUTO_INCREMENT"
		}

		createSchemaSql := ""
		if migrationSchema != "" {
			createSchemaSql = fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s;", QuoteIdentifier(migrationSchema))
		}

		sqlStmt = fmt.Sprintf(`%s
CREATE TABLE %s (
	id %s,
	%s varchar(255) NOT NULL,
	PRIMARY KEY(id)
);`, createSchemaSql, migrationTableWithSchema, id, QuoteIdentifier("table_name"))
	}

	if _, err := db.Exec(sqlStmt); err != nil {
		return err
	}
	return nil
}

// getMigrationTableName returns migration table name
func quotedMigrationTableName(dialect string) (string, error) {
	if migrationTableWithSchema != "" {
		return migrationTableWithSchema, nil
	}

	if migrationSchema == "" {
		return QuoteIdentifier(migrationTable), nil
	}

	if dialect == "" {
		return "", ErrDialectNotSpecified
	}

	if dialect == DialectSQLite {
		return QuoteIdentifier(migrationTable), nil
	}

	quoter := dialect
	if dialect == DialectSnowflake {
		quoter = DialectPostgres
	}

	return fmt.Sprintf("%s.%s", QuoteIdentifier(migrationSchema), QuoteIdentifier(migrationTable)), nil
}

// Deprecated: Use New instead
func GetMigrationRecords(db *sql.DB, dialect string) ([]*MigrationRecord, error) {
	return New(db, dialect).GetMigrationRecords()
}

func (ms *MigrationSet) GetMigrationRecords(db *sql.DB, dialect string) ([]*MigrationRecord, error) {
	migSet = *ms
	return New(db, dialect).GetMigrationRecords()
}

// Find all migration records
func (m *Migrator) GetMigrationRecords() ([]*MigrationRecord, error) {
	var migrations []*Migration
	var records []*MigrationRecord

	// Skip GetMigrationRecords if there is no migration table
	if ok, err := hasMigrationTable(m.db, m.Dialect); !ok || err != nil {
		if err != nil {
			return nil, err
		}
		return records, nil
	}

	mtx, err := quotedMigrationTableName(m.Dialect)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("SELECT %s FROM %s ORDER BY id ASC", QuoteIdentifier("table_name"), mtx)
	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var record MigrationRecord
		var id string
		var tableID string
		var temp int64

		if err = rows.Scan(&tableID); err != nil {
			return nil, err
		}

		// Find the migration number in the tableID
		re := regexp.MustCompile("[0-9]+")
		matches := re.FindAllString(tableID, -1)
		if len(matches) == 0 {
			return nil, fmt.Errorf("no version number found in %s", tableID)
		}

		id = matches[0]
		temp, err = strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing version number from %s: %w", tableID, err)
		}
		record.Id = temp
		record.AppliedAt = time.Now()
		migrations = append(migrations, &Migration{
			Id: record.Id,
		})
		records = append(records, &record)
	}

	return records, nil
}

// Exec a SQL script
func ExecParsed(db *sql.DB, script *ParsedMigration, direction MigrationDirection) error {
	// All the queries in the migration should be run in a single transaction
	txn, err := db.Begin()
	if err != nil {
		return err
	}

	for _, query := range script.Queries {
		if len(strings.TrimSpace(query)) == 0 {
			continue
		}

		if _, err := txn.Exec(query); err != nil {
			txn.Rollback()
			return fmt.Errorf("error executing statement: %s\n%w", query, err)
		}
	}

	if err := txn.Commit(); err != nil {
		return err
	}

	if direction == Up {
		if _, err := db.Exec(buildInsertMigrationQuery(script.FileName)); err != nil {
			return err
		}
	} else if direction == Down {
		if _, err := db.Exec(buildDeleteMigrationQuery(script.FileName)); err != nil {
			return err
		}
	}

	return nil
}

var dialectMap = map[string]gorp.Dialect{
	"sqlite3":    gorp.SqliteDialect{},
	"mysql":      gorp.MySQLDialect{Engine: "InnoDB", Encoding: "UTF8"},
	"postgres":   gorp.PostgresDialect{},
	"mssql":      gorp.SqlServerDialect{},
	"oci8":       gorp.OracleDialect{},
}

// GetDialect gets the gorp.Dialect based on the driver name
func GetDialect(driverName string) (gorp.Dialect, error) {
	if dialect, ok := dialectMap[driverName]; ok {
		return dialect, nil
	}
	return nil, fmt.Errorf("driver %q not supported by sql-migrate", driverName)
}

func buildInsertMigrationQuery(tableName string) string {
	quotedTableName := QuoteLiteral(tableName)
	migrationTable, _ := quotedMigrationTableName("") // Ignore the error here by using default schema name

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		migrationTable,
		QuoteIdentifier("table_name"),
		quotedTableName,
	)
}

func buildDeleteMigrationQuery(tableName string) string {
	quotedTableName := QuoteLiteral(tableName)
	migrationTable, _ := quotedMigrationTableName("") // Ignore the error here by using default schema name

	return fmt.Sprintf("DELETE FROM %s WHERE %s = %s;",
		migrationTable,
		QuoteIdentifier("table_name"),
		quotedTableName,
	)
}

// Skip a migration
func SkipMigration(db *sql.DB, direction MigrationDirection, fileName string) error {
	if direction == Up {
		if _, err := db.Exec(buildInsertMigrationQuery(fileName)); err != nil {
			return err
		}
	} else if direction == Down {
		if _, err := db.Exec(buildDeleteMigrationQuery(fileName)); err != nil {
			return err
		}
	}

	return nil
}

// Look for migration scripts with names in the form:
//
//	XXX_descriptive_name.ext
//
// where XXX specifies the version number
// and ext specifies the type of migration
func NumericComponent(fileName string) (int64, error) {
	base := path.Base(fileName)

	if ext := path.Ext(base); ext != ".go" && ext != ".sql" {
		return 0, errors.New("not a recognized migration file type")
	}

	idx := strings.Index(base, "_")
	if idx < 0 {
		return 0, errors.New("no numeric component found")
	}

	n, err := strconv.ParseInt(base[:idx], 10, 64)
	if err != nil {
		return 0, err
	}

	return n, nil
}

// Migrations from a directory.
func CollectMigrations(dirPath string, current, target int64) (migrations, error) {
	if current < 0 || target < 0 {
		return nil, errors.New("can not create a migration set with a negative value")
	}

	var migrationSet migrations

	// Make sure the dirPath ends with a trailing "/"
	dirPath = strings.TrimRight(dirPath, string(os.PathSeparator)) + string(os.PathSeparator)

	// Filter out files we're looking for in the directory.
	sqlFiles, err := filepath.Glob(dirPath + "*.sql")
	if err != nil {
		return nil, err
	}
	goFiles, err := filepath.Glob(dirPath + "*.go")
	if err != nil {
		return nil, err
	}

	var files []string
	files = append(files, sqlFiles...)
	files = append(files, goFiles...)

	for _, file := range files {
		ver, err := NumericComponent(file)
		if err != nil {
			continue
		}

		// Skip migrations that are out of the range
		if target < current {
			// Down Migrations
			if ver <= target || ver > current {
				continue
			}
		} else if current < target {
			// Up Migrations
			if ver <= current || ver > target {
				continue
			}
		} else if target == current && target > 0 {
			// Single migration
			if ver != target {
				continue
			}
		}

		migrationSet = append(migrationSet, migration{
			Version: ver,
			Source:  file,
		})
	}

	// Sort migrations
	sort.Sort(migrationSet)

	return migrationSet, nil
}

// ErrNoMigrationFiles is returned when no migration files are found.
var ErrNoMigrationFiles = errors.New("no migration files found")

// CollectAll collects all migrations in path.
func CollectAll(dirPath string, fileType string) (migrations, error) {
	// Make sure the dirPath ends with a trailing "/"
	dirPath = strings.TrimRight(dirPath, string(os.PathSeparator)) + string(os.PathSeparator)

	var migrationSet migrations

	pattern := dirPath + "*." + fileType
	// Collect all eligible files in the directory.
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, ErrNoMigrationFiles
	}

	for _, file := range files {
		ver, err := NumericComponent(file)
		if err != nil {
			continue
		}

		migrationSet = append(migrationSet, migration{
			Version: ver,
			Source:  file,
		})
	}

	// Sort migrations
	sort.Sort(migrationSet)

	return migrationSet, nil
}

// ErrNoCurrentVersion is returned when current version is nil.
var ErrNoCurrentVersion = errors.New("no current version found")

// GetMigrationVersion returns the current migration version.
func GetMigrationVersion(db *sql.DB, dialect string) (int64, error) {
	mtx, err := quotedMigrationTableName(dialect)
	if err != nil {
		return 0, err
	}

	var exists bool
	exists, err = hasMigrationTable(db, dialect)
	if err != nil {
		return 0, err
	}

	if !exists {
		return 0, nil
	}

	// Find the newest applied migration
	query := fmt.Sprintf("SELECT %s FROM %s ORDER BY id DESC LIMIT 1", QuoteIdentifier("table_name"), mtx)
	var tableName string
	err = db.QueryRow(query).Scan(&tableName)
	if err != nil {
		return 0, err
	}

	// Extract version number from the migration's name
	return NumericComponent(tableName)
}

// Deprecated: Use New().Exec(...) instead.
func Exec(db *sql.DB, dialect string, dir string, target int64) (int, error) {
	m := New(db, dialect)
	return m.Exec(dir, target)
}

// Deprecated: Use New().Migrate(...) instead.
func Migrate(db *sql.DB, dialect string, source MigrationSource, dir MigrationDirection) (int, error) {
	m := New(db, dialect)
	return m.Migrate(source, dir)
}

// Deprecated: Use New().ExecMax(...) instead.
func ExecMax(db *sql.DB, dialect string, dir string, target int64, max int) (int, error) {
	m := New(db, dialect)
	return m.ExecMax(dir, target, max)
}

// Deprecated: Use New().MigrateMax(...) instead.
func MigrateMax(db *sql.DB, dialect string, source MigrationSource, dir MigrationDirection, max int) (int, error) {
	m := New(db, dialect)
	return m.MigrateMax(source, dir, max)
}

// Deprecated: Use New().PlanMigration(...) instead.
func PlanMigration(db *sql.DB, dialect string, dir string, count int) ([]*PlannedMigration, error) {
	m := New(db, dialect)
	return m.PlanMigration(dir, count)
}

// Deprecated: Use New().Options() instead.
func SetDisableCreateTable(disable bool) {
	disableCreateTable = disable
}

// Common initialization for major migration functions
func validateMigrationSet(direction MigrationDirection, migrations migrations) error {
	if len(migrations) == 0 {
		return ErrNoMigrationFiles
	}

	// Make sure all the Go migration functions are defined
	for _, migration := range migrations {
		if filepath.Ext(migration.Source) != ".go" {
			continue
		}

		// Missing two functions used for Go migration (both up and down)?
		if len(migration.UpFn) == 0 {
			return fmt.Errorf("missing Up function for migration: %s", migration.Source)
		}

		if len(migration.DownFn) == 0 {
			return fmt.Errorf("missing Down function for migration: %s", migration.Source)
		}

		if direction == Up && migration.Up == nil {
			return fmt.Errorf("missing up function for migration: %s", migration.Source)
		}

		if direction == Down && migration.Down == nil {
			return fmt.Errorf("missing down function for migration: %s", migration.Source)
		}
	}
	return nil
}

// Execute a set of migrations
//
// Returns the number of applied migrations.
func Exec(db *sql.DB, dialect string, dir string, target int64) (int, error) {
	return ExecMax(db, dialect, dir, target, 0)
}

// Execute a set of migrations
//
// Will apply at most `max` migrations. Pass 0 for no limit (or use Exec).
//
// Returns the number of applied migrations.
func ExecMax(db *sql.DB, dialect string, dir string, target int64, max int) (int, error) {
	migrations, dbMap, err := PlanMigration(db, dialect, dir, target, max)
	if err != nil {
		return 0, err
	}

	// Apply migrations
	applied := 0
	for _, migration := range migrations {
		var executor SqlExecutor

		if migration.DisableTransaction {
			executor = dbMap
		} else {
			transaction, err := dbMap.Begin()
			if err != nil {
				return applied, err
			}

			executor = transaction
		}

		for _, stmt := range migration.Queries {
			if _, err := executor.Exec(stmt); err != nil {
				if trans, ok := executor.(*gorp.Transaction); ok {
					_ = trans.Rollback()
				}

				return applied, err
			}
		}

		if migration.DisableTransaction {
			applied++
		} else {
			transaction := executor.(*gorp.Transaction)
			if err := transaction.Commit(); err != nil {
				return applied, err
			}

			applied++
		}
	}

	return applied, nil
}

// Plan a migration.
func PlanMigration(db *sql.DB, dialect string, dir string, target int64, max int) ([]*PlannedMigration, *gorp.DbMap, error) {
	dbMap, err := getMigrationDbMap(db, dialect)
	if err != nil {
		return nil, nil, err
	}

	// Make sure migration table exists
	if err := dbMap.CreateTablesIfNotExists(); err != nil {
		return nil, nil, err
	}

	// Get current migration's version
	currentMigrationVersion, err := dbMap.SelectInt("SELECT id FROM " + dbMap.Dialect.QuotedTableForQuery(migrationTable) + " ORDER BY id DESC LIMIT 1")
	if err != nil {
		return nil, nil, err
	}

	// Figure out which direction we're going
	var direction MigrationDirection
	if currentMigrationVersion < target {
		direction = Up
	} else {
		direction = Down
	}

	// Get migrations in right direction
	var migrationSet []*Migration
	if direction == Up {
		// Up migrations
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("directory does not exist: %s", dir)
		}

		fileInfos, err := ioutil.ReadDir(dir)
		if err != nil {
			return nil, nil, err
		}

		for _, fileInfo := range fileInfos {
			if fileInfo.IsDir() {
				continue
			}

			if !strings.HasSuffix(fileInfo.Name(), ".sql") {
				continue
			}

			// Get version from filename
			versionStr := fileInfo.Name()
			versionStr = strings.TrimSuffix(versionStr, ".sql")
			version, err := strconv.ParseInt(versionStr, 10, 64)
			if err != nil {
				continue
			}

			// Only apply migrations in the right direction
			if version <= currentMigrationVersion || version > target {
				continue
			}

			// Read migration file
			file, err := os.Open(path.Join(dir, fileInfo.Name()))
			if err != nil {
				return nil, nil, err
			}
			defer file.Close()

			migrationBytes, err := ioutil.ReadAll(file)
			if err != nil {
				return nil, nil, err
			}

			migration := &Migration{
				Id:   version,
				Up:   migrationBytes,
				Down: nil,
			}
			migrationSet = append(migrationSet, migration)
		}
	} else {
		// Down migrations
		var existingMigrations []*Migration
		_, err := dbMap.Select(&existingMigrations, "SELECT * FROM "+dbMap.Dialect.QuotedTableForQuery(migrationTable)+" ORDER BY id DESC")
		if err != nil {
			return nil, nil, err
		}

		// Find all down migrations
		for _, existingMigration := range existingMigrations {
			if existingMigration.Id <= target || existingMigration.Id > currentMigrationVersion {
				continue
			}

			// Read migration file
			fileInfos, err := ioutil.ReadDir(dir)
			if err != nil {
				return nil, nil, err
			}

			for _, fileInfo := range fileInfos {
				if fileInfo.IsDir() {
					continue
				}

				if !strings.HasSuffix(fileInfo.Name(), ".sql") {
					continue
				}

				// Get version from filename
				versionStr := fileInfo.Name()
				versionStr = strings.TrimSuffix(versionStr, ".sql")
				version, err := strconv.ParseInt(versionStr, 10, 64)
				if err != nil {
					continue
				}

				// Only pick the right migration
				if version != existingMigration.Id {
					continue
				}

				// Read migration file
				file, err := os.Open(path.Join(dir, fileInfo.Name()))
				if err != nil {
					return nil, nil, err
				}
				defer file.Close()

				migrationBytes, err := ioutil.ReadAll(file)
				if err != nil {
					return nil, nil, err
				}

				migration := &Migration{
					Id:   version,
					Up:   nil,
					Down: migrationBytes,
				}
				migrationSet = append(migrationSet, migration)
				break
			}
		}
	}

	// Sort migrations
	if direction == Up {
		sort.Sort(byId(migrationSet))
	} else {
		sort.Sort(sort.Reverse(byId(migrationSet)))
	}

	// Apply a limit
	if max > 0 && len(migrationSet) > max {
		migrationSet = migrationSet[:max]
	}

	// Plan migrations
	var plannedMigrations []*PlannedMigration
	for _, migration := range migrationSet {
		var queries []string
		var sqlBytes []byte

		if direction == Up {
			sqlBytes = migration.Up
		} else {
			sqlBytes = migration.Down
		}

		// Split into individual statements
		queryParts := bytes.Split(sqlBytes, []byte(";"))
		for _, queryPart := range queryParts {
			queryPart = bytes.TrimSpace(queryPart)
			if len(queryPart) == 0 {
				continue
			}

			queries = append(queries, string(queryPart))
		}

		plannedMigration := &PlannedMigration{
			Id:                  migration.Id,
			Queries:             queries,
			DisableTransaction:  strings.Contains(string(sqlBytes), "-- disable_transaction"),
		}

		plannedMigrations = append(plannedMigrations, plannedMigration)
	}

	return plannedMigrations, dbMap, nil
}

// Returns a DbMap configured to use the migrations table
func getMigrationDbMap(db *sql.DB, dialect string) (*gorp.DbMap, error) {
	var d gorp.Dialect
	switch dialect {
	case "mysql":
		d = gorp.MySQLDialect{Engine: "InnoDB", Encoding: "UTF8"}
	case "postgres":
		d = gorp.PostgresDialect{}
	case "sqlite3":
		d = gorp.SqliteDialect{}
	case "mssql":
		d = gorp.SqlServerDialect{}
	default:
		return nil, fmt.Errorf("unsupported dialect: %s", dialect)
	}

	// Create DbMap
	dbMap := &gorp.DbMap{Db: db, Dialect: d}
	table := dbMap.AddTableWithName(Migration{}, migrationTable)
	table.SetKeys(true, "Id")
	table.ColMap("Id").Rename("id")
	table.ColMap("Up").Rename("up")
	table.ColMap("Down").Rename("down")

	return dbMap, nil
}

// Migration defines database migration
type Migration struct {
	Id   int64  `db:"id"`
	Up   []byte `db:"up"`
	Down []byte `db:"down"`
}

// PlannedMigration represents a plan to apply a migration
type PlannedMigration struct {
	Id                  int64
	Queries             []string
	DisableTransaction  bool
}

// byId sorts migration by Id ascending
type byId []*Migration

func (b byId) Len() int           { return len(b) }
func (b byId) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byId) Less(i, j int) bool { return b[i].Id < b[j].Id }

// SqlExecutor has the same methods as sql.DB and sql.Tx
type SqlExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// var used to disable creating the table
var disableCreateTable bool

// SetDisableCreateTable disables creating the migrations table
func SetDisableCreateTable(value bool) {
	disableCreateTable = value
}
EOF

echo "===========> Cleaning complete"
chmod +x hack/clean-vendor.sh
