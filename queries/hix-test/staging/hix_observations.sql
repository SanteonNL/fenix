-- :pk observation_id
SELECT
    observation_id,
    patient_id,
    obs_status,
    CONVERT(VARCHAR(10), obs_date, 120)    AS obs_date,
    obs_value,
    obs_unit,
    obs_type,
    CONVERT(VARCHAR(23), updated_at, 126)  AS updated_at
FROM hix_observations
{{- if .since}} WHERE updated_at > '{{.since}}'{{end}}
