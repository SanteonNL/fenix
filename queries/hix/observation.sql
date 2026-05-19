-- Observation SQL-to-FHIR query
-- Meerdere statements, elk een aparte resultset.
-- Meerdere rijen voor dezelfde fhir_path + parent_id → FHIR array.
--
-- Verwachte CSV-tabellen:
--   observations.csv          → tabel "observations"
--   observation_category.csv  → tabel "observation_category"
--   observation_coding.csv    → tabel "observation_coding"
--
-- Voorbeeld: TNM-staging observatie met meerdere categorieën en codes.

-- ── Statement 1: Root Observation ─────────────────────────────────────────
-- observations.csv kolommen: observation_id, patient_id, status, effective_date, value_value, value_unit
SELECT
    observation_id                      AS resource_id,
    observation_id                      AS id,
    ''                                  AS parent_id,
    'Observation'                       AS fhir_path,
    status,
    effective_date                      AS effectiveDateTime,
    'Patient/' || patient_id            AS "subject.reference",
    value_value                         AS "valueQuantity.value",
    value_unit                          AS "valueQuantity.unit"
FROM observations;

-- ── Statement 2: Category ─────────────────────────────────────────────────
-- observation_category.csv kolommen: observation_id, category_id, category_text
-- Meerdere rijen per observation_id → array in Observation.category
SELECT
    observation_id                      AS resource_id,
    category_id                         AS id,
    observation_id                      AS parent_id,
    'Observation.category'              AS fhir_path,
    category_text                       AS text
FROM observation_category;

-- ── Statement 3: Category coding ─────────────────────────────────────────
-- observation_category_coding.csv kolommen: category_id, coding_id, coding_system, coding_code, coding_display
-- Meerdere rijen per category_id → array in Observation.category.coding
SELECT
    c.observation_id                    AS resource_id,
    cc.coding_id                        AS id,
    cc.category_id                      AS parent_id,
    'Observation.category.coding'       AS fhir_path,
    cc.coding_system                    AS "system",
    cc.coding_code                      AS code,
    cc.coding_display                   AS display
FROM observation_category_coding cc
JOIN observation_category c ON c.category_id = cc.category_id;

-- ── Statement 4: Code (LOINC/SNOMED) ──────────────────────────────────────
-- observation_coding.csv kolommen: observation_id, coding_id, coding_system, coding_code, coding_display
-- Meerdere rijen per observation_id → array in Observation.code.coding
SELECT
    observation_id                      AS resource_id,
    coding_id                           AS id,
    observation_id                      AS parent_id,
    'Observation.code.coding'           AS fhir_path,
    coding_system                       AS "system",
    coding_code                         AS code,
    coding_display                      AS display
FROM observation_coding;
