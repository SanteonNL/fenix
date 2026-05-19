-- Patient SQL-to-FHIR query (hix-test / SQL Server)
-- Table names injected via substitutions: {{.patient_table}}, {{.patient_names_table}}, etc.

-- ── Statement 1: Root Patient ──────────────────────────────────────────────
SELECT
    patient_id      AS resource_id,
    patient_id      AS id,
    ''              AS parent_id,
    'Patient'       AS fhir_path,
    gender,
    CONVERT(VARCHAR(10), birth_date, 120) AS birthDate,
    active
FROM {{.patient_table}}
WHERE 1=1
{{- if .birthDate_from}} AND birth_date >= '{{.birthDate_from}}'{{end}}
{{- if .birthDate_to}}   AND birth_date <= '{{.birthDate_to}}'{{end}}
{{- if .gender}}         AND gender = '{{.gender}}'{{end}}
{{- if .extra_where}}    AND {{.extra_where}}{{end}};

-- ── Statement 2: Name ─────────────────────────────────────────────────────
SELECT
    patient_id      AS resource_id,
    name_id         AS id,
    patient_id      AS parent_id,
    'Patient.name'  AS fhir_path,
    name_use        AS "use",
    family,
    given
FROM {{.patient_names_table}};

-- ── Statement 3: Telecom ──────────────────────────────────────────────────
SELECT
    patient_id          AS resource_id,
    telecom_id          AS id,
    patient_id          AS parent_id,
    'Patient.telecom'   AS fhir_path,
    telecom_system      AS "system",
    telecom_value       AS "value",
    telecom_use         AS "use"
FROM {{.patient_telecom_table}};

-- ── Statement 4: Address ─────────────────────────────────────────────────
SELECT
    patient_id          AS resource_id,
    address_id          AS id,
    patient_id          AS parent_id,
    'Patient.address'   AS fhir_path,
    address_use         AS "use",
    line,
    city,
    postal_code         AS postalCode,
    country
FROM {{.patient_address_table}};

-- ── Statement 5: Identifier ───────────────────────────────────────────────
SELECT
    patient_id              AS resource_id,
    identifier_id           AS id,
    patient_id              AS parent_id,
    'Patient.identifier'    AS fhir_path,
    id_use                  AS "use",
    id_system               AS "system",
    id_value                AS "value"
FROM {{.patient_identifier_table}};
