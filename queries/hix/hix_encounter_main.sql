-- HIX Encounter query (main)
-- Template vars: .period_from / .period_to  (FHIR date param → Encounter.period)
--                .status                    (FHIR status param)
--                .extra_where               (group config injection)

-- ── Statement 1: Root Encounter ───────────────────────────────────────────
SELECT
    encounter_id                        AS resource_id,
    encounter_id                        AS id,
    ''                                  AS parent_id,
    'Encounter'                         AS fhir_path,
    status,
    start_time                          AS "period.start",
    end_time                            AS "period.end",
    'Patient/' || patient_id            AS "subject.reference",
    service_provider                    AS "serviceProvider.display"
FROM {{.encounter_table}}
WHERE 1=1
{{- if .period_from}} AND start_time >= '{{.period_from}}'{{end}}
{{- if .period_to}}   AND start_time <= '{{.period_to}}'{{end}}
{{- if .status}}      AND status = '{{.status}}'{{end}}
{{- if .extra_where}} AND {{.extra_where}}{{end}};

-- ── Statement 2: Type ─────────────────────────────────────────────────────
SELECT
    encounter_id            AS resource_id,
    type_id                 AS id,
    encounter_id            AS parent_id,
    'Encounter.type'        AS fhir_path,
    type_text               AS text
FROM {{.encounter_type_table}};

-- ── Statement 3: Type coding ──────────────────────────────────────────────
SELECT
    t.encounter_id              AS resource_id,
    tc.coding_id                AS id,
    tc.type_id                  AS parent_id,
    'Encounter.type.coding'     AS fhir_path,
    tc.coding_system            AS "system",
    tc.coding_code              AS code,
    tc.coding_display           AS display
FROM {{.encounter_type_coding_table}} tc
JOIN {{.encounter_type_table}} t ON t.type_id = tc.type_id;

-- ── Statement 4: Reason ───────────────────────────────────────────────────
SELECT
    encounter_id            AS resource_id,
    reason_id               AS id,
    encounter_id            AS parent_id,
    'Encounter.reason'      AS fhir_path,
    reason_system           AS "system",
    reason_code             AS code,
    reason_display          AS display
FROM {{.encounter_reason_table}};

-- ── Statement 5: Status history ───────────────────────────────────────────
SELECT
    encounter_id            AS resource_id,
    history_id              AS id,
    encounter_id            AS parent_id,
    'Encounter.statusHistory' AS fhir_path,
    status,
    period_start            AS "period.start",
    period_end              AS "period.end"
FROM {{.encounter_status_history_table}};
