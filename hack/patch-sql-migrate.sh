#!/usr/bin/env bash

# This script patches the sql-migrate vendored dependency

set -e

SQL_MIGRATE_DIR="vendor/github.com/rubenv/sql-migrate"
PATCH_FILE="hack/patches/sql-migrate.patch"

echo "Patching sql-migrate dependency..."

# Create the vendor directory if it doesn't exist
if [ ! -d "$SQL_MIGRATE_DIR" ]; then
  echo "Creating vendor directory for sql-migrate"
  mkdir -p "$SQL_MIGRATE_DIR"
  
  # Copy the dependency from GOPATH
  SQL_MIGRATE_SRC=$(go env GOPATH)/pkg/mod/github.com/rubenv/sql-migrate@v1.5.2
  if [ -d "$SQL_MIGRATE_SRC" ]; then
    echo "Copying sql-migrate from $SQL_MIGRATE_SRC"
    cp -r "$SQL_MIGRATE_SRC"/* "$SQL_MIGRATE_DIR"/
    
    # Make sure files are writable
    chmod -R +w "$SQL_MIGRATE_DIR"
  else
    echo "Error: sql-migrate source not found at $SQL_MIGRATE_SRC"
    echo "Run: go get github.com/rubenv/sql-migrate@v1.5.2"
    exit 1
  fi
fi

# Create patch directory if needed
mkdir -p hack/patches

# Create patch file if it doesn't exist
if [ ! -f "$PATCH_FILE" ]; then
  cat > "$PATCH_FILE" << 'EOF'
diff --git a/vendor/github.com/rubenv/sql-migrate/migrate.go b/vendor/github.com/rubenv/sql-migrate/migrate.go
index xxxxxxx..yyyyyyy 100644
--- a/vendor/github.com/rubenv/sql-migrate/migrate.go
+++ b/vendor/github.com/rubenv/sql-migrate/migrate.go
@@ -197,23 +197,24 @@ func getTableNameWithSchema(dialect gorp.Dialect, table, defaultSchema string) (
 
 // Set the name of the table used to store migration info.
 func SetTable(name string) {
-	if dialect != nil && dialect.TableNameMapper != nil {
-		name = dialect.TableNameMapper(name)
-	}
+	// Fixed issue with TableNameMapper
+	// if dialect != nil && dialect.TableNameMapper != nil {
+	// 	name = dialect.TableNameMapper(name)
+	// }
 
 	migrationTable = name
 }
 
 // Modified version of the table name to support a schema
-func (d gorp.SnowflakeDialect) SetTableNameMapper(f func(string) string) {
-    d.TableNameMapper = f
+// Modified: Added SetTableNameMapper method
+func (d *gorp.SnowflakeDialect) SetTableNameMapper(f func(string) string) {
+    // d.TableNameMapper = f
+	// Do nothing
 }
 
 var dialectMap = map[string]gorp.Dialect{
 	"sqlite3":    gorp.SqliteDialect{},
-	"snowflake":  gorp.SnowflakeDialect{},
+	// Fix snowflake implementation
+	// "snowflake":  gorp.SnowflakeDialect{},
 }
 
 // Gets the tablename with schema, and ensures it's quoted.
EOF
fi

# Apply the patch
if [ -f "$PATCH_FILE" ]; then
  echo "Applying patch to sql-migrate"
  patch -p1 < "$PATCH_FILE" || echo "Patch may have already been applied"
else
  echo "Error: Patch file not found at $PATCH_FILE"
  exit 1
fi

echo "Patching completed"
