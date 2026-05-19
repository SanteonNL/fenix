-- :pk vital_id
SELECT
    vital_id,
    patient_id,
    vital_status,
    CONVERT(VARCHAR(23), measured_at, 126) AS measured_at,
    vital_value,
    vital_unit,
    loinc_code,
    loinc_display,
    CONVERT(VARCHAR(23), updated_at, 126)  AS updated_at
FROM hix_vitals
{{- if .since}} WHERE updated_at > '{{.since}}'{{end}}
