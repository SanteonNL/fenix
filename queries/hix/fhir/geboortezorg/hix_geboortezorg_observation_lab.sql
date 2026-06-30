-- Geboortezorg lab-result Observation query (HIX source)
-- Scoped to the maternity care department lab table.
-- Template vars: .effective_from / .effective_to, .code, .extra_where (set by group config)

-- ── Statement 1: Root Observation ─────────────────────────────────────────
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
FROM hix_verloskunde_lab
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
FROM hix_verloskunde_lab
WHERE 1=1
{{- if .effective_from}} AND result_date >= '{{.effective_from}}'{{end}}
{{- if .effective_to}}   AND result_date <= '{{.effective_to}}'{{end}};
