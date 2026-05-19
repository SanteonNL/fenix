-- HIX vital-signs Observation query (SQL Server / T-SQL)
-- Template vars: .effective_from / .effective_to, .status, .extra_where

-- ── Statement 1: Root Observation (vital sign) ─────────────────────────────
SELECT
    vital_id                        AS resource_id,
    vital_id                        AS id,
    ''                              AS parent_id,
    'Observation'                   AS fhir_path,
    vital_status                    AS status,
    measured_at                     AS effectiveDateTime,
    'Patient/' + patient_id         AS "subject.reference",
    vital_value                     AS "valueQuantity.value",
    vital_unit                      AS "valueQuantity.unit"
FROM hix_vitals
WHERE 1=1
{{- if .effective_from}} AND measured_at >= '{{.effective_from}}'{{end}}
{{- if .effective_to}}   AND measured_at <= '{{.effective_to}}'{{end}}
{{- if .status}}         AND vital_status = '{{.status}}'{{end}}
{{- if .extra_where}}    AND {{.extra_where}}{{end}};

-- ── Statement 2: Category (always "vital-signs") ──────────────────────────
SELECT
    vital_id                    AS resource_id,
    vital_id + '_cat'           AS id,
    vital_id                    AS parent_id,
    'Observation.category'      AS fhir_path,
    'Vital Signs'               AS text
FROM hix_vitals
WHERE 1=1
{{- if .effective_from}} AND measured_at >= '{{.effective_from}}'{{end}}
{{- if .effective_to}}   AND measured_at <= '{{.effective_to}}'{{end}};

-- ── Statement 3: Category coding ─────────────────────────────────────────
SELECT
    vital_id                            AS resource_id,
    vital_id + '_cat_coding'            AS id,
    vital_id + '_cat'                   AS parent_id,
    'Observation.category.coding'       AS fhir_path,
    'http://terminology.hl7.org/CodeSystem/observation-category' AS "system",
    'vital-signs'                       AS code,
    'Vital Signs'                       AS display
FROM hix_vitals
WHERE 1=1
{{- if .effective_from}} AND measured_at >= '{{.effective_from}}'{{end}}
{{- if .effective_to}}   AND measured_at <= '{{.effective_to}}'{{end}};

-- ── Statement 4: Code (LOINC per vital type) ──────────────────────────────
SELECT
    vital_id                    AS resource_id,
    vital_id + '_code'          AS id,
    vital_id                    AS parent_id,
    'Observation.code.coding'   AS fhir_path,
    'http://loinc.org'          AS "system",
    loinc_code                  AS code,
    loinc_display               AS display
FROM hix_vitals
WHERE 1=1
{{- if .effective_from}} AND measured_at >= '{{.effective_from}}'{{end}}
{{- if .effective_to}}   AND measured_at <= '{{.effective_to}}'{{end}};
