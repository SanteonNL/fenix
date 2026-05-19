# setup_db.R
# Pipeline: FHIR NDJSON -> flatquack SQL -> DuckDB tables
#
# Usage: Rscript setup_db.R
#   Generates output/fhir.duckdb ready for analysis.

.libPaths(c("C:/Users/t.hetterscheid/R/library", .libPaths()))

library(DBI)
library(duckdb)

args <- commandArgs(trailingOnly = FALSE)
script_flag <- grep("^--file=", args, value = TRUE)
if (length(script_flag)) {
  BASE_DIR <- normalizePath(dirname(sub("^--file=", "", script_flag)), winslash = "/")
} else {
  BASE_DIR <- normalizePath(".", winslash = "/")
}

VIEWS_DIR  <- file.path(BASE_DIR, "views")
NDJSON_DIR <- file.path(BASE_DIR, "ndjson")
OUTPUT_DIR <- file.path(BASE_DIR, "output")
DB_PATH    <- file.path(OUTPUT_DIR, "fhir.duckdb")

dir.create(OUTPUT_DIR, showWarnings = FALSE, recursive = TRUE)

# ── 1. Run flatquack to (re)generate SQL files ────────────────────────────────
cat(">> Running flatquack --mode build ...\n")
flatquack_cmd <- paste(
  "flatquack",
  "--mode build",
  paste0('--view-path "', VIEWS_DIR, '"'),
  "--view-pattern **/*.vd.json",
  "--template @csv"
)
ret <- system(flatquack_cmd)
if (ret != 0) {
  message("flatquack exited non-zero (", ret, "). SQL files may still be usable.")
}

# ── 2. Open DuckDB and load data via adapted SQL ──────────────────────────────
cat(">> Opening DuckDB:", DB_PATH, "\n")
if (file.exists(DB_PATH)) file.remove(DB_PATH)
con <- dbConnect(duckdb(), dbdir = DB_PATH)

# Ensure the json extension is available
tryCatch({
  dbExecute(con, "INSTALL json")
  dbExecute(con, "LOAD json")
  cat("  json extension loaded\n")
}, error = function(e) {
  tryCatch(dbExecute(con, "LOAD json"),
           error = function(e2) message("json extension not available: ", e2$message))
})

# Register DuckDB macros used by flatquack-generated SQL
macros_sql <- "
CREATE OR REPLACE MACRO as_list(a)    AS if(a IS NULL, [], [a]);
CREATE OR REPLACE MACRO ifnull2(a, b) AS ifnull(a, b);
CREATE OR REPLACE MACRO slice(a, i)   AS a[i];
CREATE OR REPLACE MACRO is_false(a)   AS a = false;
CREATE OR REPLACE MACRO is_true(a)    AS a = true;
CREATE OR REPLACE MACRO is_null(a)    AS a IS NULL;
CREATE OR REPLACE MACRO is_not_null(a) AS a IS NOT NULL;
CREATE OR REPLACE MACRO as_value(a)   AS if(
  len(a) > 1,
  error('unexpected collection returned'),
  a[1]
);
"
for (stmt in strsplit(macros_sql, ";")[[1]]) {
  stmt <- trimws(stmt)
  if (nzchar(stmt)) dbExecute(con, stmt)
}

# Helper: extract the inner SELECT from a flatquack COPY statement,
# then create a table with that SELECT.
load_view <- function(sql_file, table_name) {
  cat("  Loading", table_name, "from", basename(sql_file), "...\n")
  raw <- paste(readLines(sql_file, warn = FALSE), collapse = "\n")

  # Strip the macro block (ends before COPY)
  copy_pos <- regexpr("COPY\\s*\\(", raw, perl = TRUE)
  if (copy_pos == -1) {
    warning("No COPY statement found in ", sql_file)
    return(invisible(NULL))
  }
  after_copy <- substring(raw, copy_pos + attr(copy_pos, "match.length") - 1)

  # Find the matching closing paren for the COPY ( ... )
  depth <- 0
  inner_end <- NA
  for (i in seq_len(nchar(after_copy))) {
    ch <- substr(after_copy, i, i)
    if (ch == "(") depth <- depth + 1
    if (ch == ")") {
      depth <- depth - 1
      if (depth == 0) { inner_end <- i; break }
    }
  }
  inner_select <- substring(after_copy, 2, inner_end - 1)

  # Fix the absolute NDJSON path: flatquack hardcodes the CWD at build time.
  # Replace any **/*<Resource>*.ndjson glob with our actual ndjson dir.
  inner_select <- gsub(
    "read_json_auto\\(\\s*'[^']*[/\\\\]\\*\\*[/\\\\]\\*([A-Za-z]+)\\*\\.ndjson'",
    paste0("read_json_auto('", NDJSON_DIR, "/*\\1*.ndjson'"),
    inner_select, perl = TRUE
  )
  # Also normalise any remaining backslashes in paths to forward slashes
  inner_select <- gsub("\\\\", "/", inner_select)

  create_sql <- paste0(
    "CREATE OR REPLACE TABLE ", table_name, " AS (\n",
    inner_select, "\n);"
  )

  ok <- tryCatch({
    dbExecute(con, create_sql)
    TRUE
  }, error = function(e) {
    warning("Error loading ", table_name, ": ", conditionMessage(e))
    FALSE
  })

  if (ok) {
    n <- dbGetQuery(con, paste("SELECT count(*) AS n FROM", table_name))$n
    cat("    ->", n, "rows inserted into", table_name, "\n")
  }
  invisible(NULL)
}

# ── 3. Load each view ─────────────────────────────────────────────────────────
sql_files <- list.files(VIEWS_DIR, pattern = "\\.vd\\.sql$", full.names = TRUE)
view_map  <- list(
  patient_flat          = "patient_flat",
  observation_flat      = "observation_flat",
  condition_flat        = "condition_flat",
  encounter_flat        = "encounter_flat",
  medication_request_flat = "medication_request_flat"
)

for (f in sql_files) {
  base <- sub("\\.vd\\.sql$", "", basename(f))
  # map file base name to view name by reading the SQL header comment or
  # using the ViewDefinition name (encoded in the COPY ... TO path)
  raw  <- paste(readLines(f, warn = FALSE), collapse = " ")
  # Extract table name from COPY (...) TO '.../NAME.csv'
  m <- regmatches(raw, regexpr("TO\\s+'[^']*?([^/\\\\]+)\\.csv'", raw, perl = TRUE))
  if (length(m)) {
    tname <- sub("TO\\s+'.*?([^/\\\\]+)\\.csv'", "\\1", m, perl = TRUE)
  } else {
    tname <- gsub("[^a-z0-9_]", "_", base)
  }
  load_view(f, tname)
}

# ── 4. Quick sanity check ─────────────────────────────────────────────────────
cat("\n>> Tables in fhir.duckdb:\n")
print(dbGetQuery(con, "SHOW TABLES"))

cat("\n>> Sample patient data:\n")
print(dbGetQuery(con, "SELECT id, family_name, given_name, gender, birth_date FROM patient_flat"))

dbDisconnect(con, shutdown = TRUE)
cat("\n>> Done. DuckDB written to:", DB_PATH, "\n")
