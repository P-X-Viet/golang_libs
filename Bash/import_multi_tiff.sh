#!/bin/bash
# =============================================================
# Import all .tif files from a folder into a PostGIS table
# Reprojects each file to EPSG:4326 using gdalwarp
# =============================================================

# ─── Configuration ───────────────────────────────────────────
INPUT_DIR="folder_a"          # Folder containing .tif files
TARGET_SRID="4326"            # Target projection
TABLE="public.raster_table"   # Target PostGIS table
TILE_SIZE="256x256"           # Tile size for raster2pgsql

DB_HOST="localhost"
DB_PORT="5432"
DB_NAME="your_database"
DB_USER="postgres"
# DB_PASS="your_password"     # Uncomment if needed (or use .pgpass)
# ─────────────────────────────────────────────────────────────

# Optional: set password via env var to avoid prompt
# export PGPASSWORD="your_password"

# ─── Validation ──────────────────────────────────────────────
if [ ! -d "$INPUT_DIR" ]; then
  echo "ERROR: Input directory '$INPUT_DIR' not found."
  exit 1
fi

TIF_COUNT=$(find "$INPUT_DIR" -name "*.tif" | wc -l)
if [ "$TIF_COUNT" -eq 0 ]; then
  echo "ERROR: No .tif files found in '$INPUT_DIR'."
  exit 1
fi

echo "Found $TIF_COUNT .tif file(s) in '$INPUT_DIR'"

# ─── Check required tools ────────────────────────────────────
for tool in gdalwarp raster2pgsql psql; do
  if ! command -v "$tool" &>/dev/null; then
    echo "ERROR: '$tool' is not installed or not in PATH."
    exit 1
  fi
done

# ─── Step 1: Create table from the first file ────────────────
FIRST_FILE=$(find "$INPUT_DIR" -name "*.tif" | sort | head -1)
echo ""
echo "[1/2] Creating table from: $FIRST_FILE"

gdalwarp -t_srs "EPSG:$TARGET_SRID" -overwrite \
  "$FIRST_FILE" /vsistdout/ -of GTiff 2>/dev/null | \
raster2pgsql \
  -s "$TARGET_SRID" \
  -I \
  -C \
  -M \
  -t "$TILE_SIZE" \
  -F \
  - "$TABLE" | \
psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME"

if [ $? -ne 0 ]; then
  echo "ERROR: Failed to create table from first file."
  exit 1
fi

echo "Table '$TABLE' created successfully."

# ─── Step 2: Append remaining files ──────────────────────────
REMAINING=$(find "$INPUT_DIR" -name "*.tif" | sort | tail -n +2)
REMAINING_COUNT=$(echo "$REMAINING" | grep -c ".")

if [ "$REMAINING_COUNT" -eq 0 ]; then
  echo "Only one file found. Import complete."
else
  echo ""
  echo "[2/2] Appending $REMAINING_COUNT remaining file(s)..."
  CURRENT=1

  echo "$REMAINING" | while read -r f; do
    echo "  ($CURRENT/$REMAINING_COUNT) Importing: $f"
    gdalwarp -t_srs "EPSG:$TARGET_SRID" -overwrite \
      "$f" /vsistdout/ -of GTiff 2>/dev/null | \
    raster2pgsql \
      -s "$TARGET_SRID" \
      -a \
      -t "$TILE_SIZE" \
      -F \
      - "$TABLE"
    CURRENT=$((CURRENT + 1))
  done | psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME"

  if [ $? -ne 0 ]; then
    echo "ERROR: Failed during append step."
    exit 1
  fi
fi

# ─── Done ─────────────────────────────────────────────────────
echo ""
echo "Import complete!"
echo ""
echo "Verify with:"
echo "  psql -U $DB_USER -d $DB_NAME -c \"SELECT filename, ST_SRID(rast) FROM $TABLE LIMIT 5;\""
