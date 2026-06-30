-- HIX lab-result Observation query
-- Lab results live in a separate table from general observations.
-- Template vars: .effective_from / .effective_to (date param), .code (code param), .extra_where

-- ── Statement 1: Root Observation (lab results) ────────────────────────────
SELECT
    lab_id                              AS resource_id,
    lab_id                              AS id,
    ''                                  AS parent_id,
    'Observation'                       AS fhir_path,
    lab_status                          AS status,
    result_date                         AS effectiveDateTime,
    'Patient/' || patient_id            AS "subject.reference",
    result_value                        AS "valueQuantity.value",
    result_unit                         AS "valueQuantity.unit"
FROM hix_lab_results
WHERE 1=1
{{- if .effective_from}} AND result_date >= '{{.effective_from}}'{{end}}
{{- if .effective_to}}   AND result_date <= '{{.effective_to}}'{{end}}
{{- if .code}}           AND loinc_code = '{{.code}}'{{end}}
{{- if .extra_where}}    AND {{.extra_where}}{{end}};

-- ── Statement 2: Code (LOINC) ──────────────────────────────────────────────
SELECT
    lab_id                          AS resource_id,
    lab_id || '_code'               AS id,
    lab_id                          AS parent_id,
    'Observation.code.coding'       AS fhir_path,
    'http://loinc.org'              AS "system",
    loinc_code                      AS code,
    loinc_display                   AS display
FROM hix_lab_results
WHERE 1=1
{{- if .effective_from}} AND result_date >= '{{.effective_from}}'{{end}}
{{- if .effective_to}}   AND result_date <= '{{.effective_to}}'{{end}};
