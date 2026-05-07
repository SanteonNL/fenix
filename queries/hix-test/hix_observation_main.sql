-- HIX general Observation query (SQL Server / T-SQL)
-- Template vars: .effective_from / .effective_to, .status, .code, .category, .extra_where

-- ── Statement 1: Root Observation ─────────────────────────────────────────
SELECT
    observation_id                      AS resource_id,
    observation_id                      AS id,
    ''                                  AS parent_id,
    'Observation'                       AS fhir_path,
    obs_status                          AS status,
    obs_date                            AS effectiveDateTime,
    'Patient/' + patient_id             AS "subject.reference",
    obs_value                           AS "valueQuantity.value",
    obs_unit                            AS "valueQuantity.unit"
FROM hix_observations
WHERE obs_type NOT IN ('LAB', 'VITAL')
{{- if .effective_from}} AND obs_date >= '{{.effective_from}}'{{end}}
{{- if .effective_to}}   AND obs_date <= '{{.effective_to}}'{{end}}
{{- if .status}}         AND obs_status = '{{.status}}'{{end}}
{{- if .extra_where}}    AND {{.extra_where}}{{end}};

-- ── Statement 2: Category ─────────────────────────────────────────────────
SELECT
    observation_id              AS resource_id,
    cat_id                      AS id,
    observation_id              AS parent_id,
    'Observation.category'      AS fhir_path,
    cat_text                    AS text
FROM hix_obs_category
WHERE 1=1
{{- if .extra_where}} AND {{.extra_where}}{{end}};

-- ── Statement 3: Category coding ─────────────────────────────────────────
SELECT
    c.observation_id                AS resource_id,
    cc.coding_id                    AS id,
    cc.cat_id                       AS parent_id,
    'Observation.category.coding'   AS fhir_path,
    cc.coding_system                AS "system",
    cc.coding_code                  AS code,
    cc.coding_display               AS display
FROM hix_obs_category_coding cc
JOIN hix_obs_category c ON c.cat_id = cc.cat_id;

-- ── Statement 4: Code (SNOMED/LOINC) ──────────────────────────────────────
SELECT
    observation_id              AS resource_id,
    coding_id                   AS id,
    observation_id              AS parent_id,
    'Observation.code.coding'   AS fhir_path,
    coding_system               AS "system",
    coding_code                 AS code,
    coding_display              AS display
FROM hix_obs_coding;
