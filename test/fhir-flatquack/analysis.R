# analysis.R
# Demonstrates querying the FHIR DuckDB created by setup_db.R
#
# Run after setup_db.R has generated output/fhir.duckdb
# Usage: Rscript analysis.R

.libPaths(c("C:/Users/t.hetterscheid/R/library", .libPaths()))

library(DBI)
library(duckdb)
library(dplyr)

args <- commandArgs(trailingOnly = FALSE)
script_flag <- grep("^--file=", args, value = TRUE)
if (length(script_flag)) {
  BASE_DIR <- normalizePath(dirname(sub("^--file=", "", script_flag)), winslash = "/")
} else {
  BASE_DIR <- normalizePath(".", winslash = "/")
}

DB_PATH <- file.path(BASE_DIR, "output", "fhir.duckdb")
stopifnot("Run setup_db.R first" = file.exists(DB_PATH))

con <- dbConnect(duckdb(), dbdir = DB_PATH, read_only = TRUE)
on.exit(dbDisconnect(con, shutdown = TRUE))

# ── Helper to pull a table as a lazy tbl ─────────────────────────────────────
tbl_fhir <- function(name) tbl(con, name)

# ── 1. Patient demographics ───────────────────────────────────────────────────
cat("\n=== 1. Patient Demographics ===\n")
current_year <- as.integer(format(Sys.Date(), "%Y"))
patients <- tbl_fhir("patient_flat") |>
  select(id, given_name, family_name, gender, birth_date, city, country) |>
  collect() |>
  mutate(
    patient_id = id,
    full_name  = paste(given_name, family_name),
    age        = current_year - as.integer(substr(birth_date, 1, 4))
  ) |>
  select(patient_id, full_name, gender, birth_date, age, city, country)
print(patients)

# ── 2. Observation summary per patient ───────────────────────────────────────
cat("\n=== 2. Observations per Patient ===\n")
obs_summary <- tbl_fhir("observation_flat") |>
  mutate(patient_id = regexp_replace(patient_ref, "Patient/", "")) |>
  group_by(patient_id, display, unit) |>
  summarise(
    n_obs      = n(),
    latest_val = max(value, na.rm = TRUE),
    .groups    = "drop"
  ) |>
  left_join(tbl_fhir("patient_flat") |> select(id, family_name), by = c("patient_id" = "id")) |>
  arrange(patient_id, display) |>
  collect()
print(obs_summary)

# ── 3. Active conditions by patient ──────────────────────────────────────────
cat("\n=== 3. Active Conditions ===\n")
conditions <- tbl_fhir("condition_flat") |>
  filter(clinical_status == "active") |>
  mutate(patient_id = regexp_replace(patient_ref, "Patient/", "")) |>
  left_join(tbl_fhir("patient_flat") |> select(id, family_name, given_name),
            by = c("patient_id" = "id")) |>
  select(patient_id, given_name, family_name, condition_display, severity, onset_date) |>
  collect()
print(conditions)

# ── 4. Medications per patient ───────────────────────────────────────────────
cat("\n=== 4. Medications ===\n")
meds <- tbl_fhir("medication_request_flat") |>
  filter(status == "active") |>
  mutate(patient_id = regexp_replace(patient_ref, "Patient/", "")) |>
  left_join(tbl_fhir("patient_flat") |> select(id, family_name),
            by = c("patient_id" = "id")) |>
  select(family_name, medication_display, dosage_text, authored_on, requester) |>
  arrange(family_name) |>
  collect() |>
  # flatquack types some fields as JSON → strip surrounding quotes
  mutate(medication_display = gsub('^"|"$', "", medication_display))
print(meds)

# ── 5. Encounter count per patient ───────────────────────────────────────────
cat("\n=== 5. Encounter Activity ===\n")
encounters <- tbl_fhir("encounter_flat") |>
  mutate(patient_id = regexp_replace(patient_ref, "Patient/", "")) |>
  count(patient_id, encounter_type, status, name = "n_encounters") |>
  left_join(tbl_fhir("patient_flat") |> select(id, family_name),
            by = c("patient_id" = "id")) |>
  arrange(desc(n_encounters)) |>
  collect()
print(encounters)

# ── 6. Cross-resource: patients with both diabetes and HbA1c ─────────────────
cat("\n=== 6. Diabetic Patients with HbA1c Results ===\n")
diabetic_ids <- tbl_fhir("condition_flat") |>
  filter(grepl("Diabetes", condition_display)) |>
  mutate(patient_id = regexp_replace(patient_ref, "Patient/", "")) |>
  select(patient_id) |>
  distinct()

hba1c <- tbl_fhir("observation_flat") |>
  filter(grepl("HbA1c|A1c|4548", display)) |>
  mutate(patient_id = regexp_replace(patient_ref, "Patient/", "")) |>
  select(patient_id, display, value, unit, effective_date)

result <- inner_join(diabetic_ids, hba1c, by = "patient_id") |>
  left_join(tbl_fhir("patient_flat") |> select(id, family_name),
            by = c("patient_id" = "id")) |>
  collect()
print(result)

cat("\nAnalysis complete.\n")
