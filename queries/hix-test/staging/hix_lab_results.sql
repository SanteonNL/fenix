-- :pk lab_id
SELECT
    lab_id,
    patient_id,
    lab_status,
    CONVERT(VARCHAR(10), result_date, 120) AS result_date,
    result_value,
    result_unit,
    loinc_code,
    loinc_display,
    CONVERT(VARCHAR(23), updated_at, 126)  AS updated_at
FROM hix_lab_results
{{- if .since}} WHERE updated_at > '{{.since}}'{{end}}
