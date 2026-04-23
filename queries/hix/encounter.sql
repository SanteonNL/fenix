-- Encounter SQL-to-FHIR query
-- Meerdere statements, elk een aparte resultset.
-- Meerdere rijen voor dezelfde fhir_path + parent_id → FHIR array.
--
-- Verwachte CSV-tabellen:
--   encounters.csv        → tabel "encounters"
--   encounter_type.csv    → tabel "encounter_type"
--   encounter_reason.csv  → tabel "encounter_reason"

-- ── Statement 1: Root Encounter ───────────────────────────────────────────
-- encounters.csv kolommen: encounter_id, patient_id, status, start_time, end_time, service_provider
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
FROM encounters;

-- ── Statement 2: Type ─────────────────────────────────────────────────────
-- encounter_type.csv kolommen: encounter_id, type_id, type_text
-- Meerdere rijen per encounter_id → array in Encounter.type
SELECT
    encounter_id                        AS resource_id,
    type_id                             AS id,
    encounter_id                        AS parent_id,
    'Encounter.type'                    AS fhir_path,
    type_text                           AS text
FROM encounter_type;

-- ── Statement 3: Type coding ──────────────────────────────────────────────
-- encounter_type_coding.csv kolommen: type_id, coding_id, coding_system, coding_code, coding_display
-- Meerdere rijen per type_id → array in Encounter.type.coding
SELECT
    t.encounter_id                      AS resource_id,
    tc.coding_id                        AS id,
    tc.type_id                          AS parent_id,
    'Encounter.type.coding'             AS fhir_path,
    tc.coding_system                    AS "system",
    tc.coding_code                      AS code,
    tc.coding_display                   AS display
FROM encounter_type_coding tc
JOIN encounter_type t ON t.type_id = tc.type_id;

-- ── Statement 4: Reason ───────────────────────────────────────────────────
-- encounter_reason.csv kolommen: encounter_id, reason_id, reason_system, reason_code, reason_display
-- Meerdere rijen per encounter_id → array in Encounter.reason
SELECT
    encounter_id                        AS resource_id,
    reason_id                           AS id,
    encounter_id                        AS parent_id,
    'Encounter.reason'                  AS fhir_path,
    reason_system                       AS "system",
    reason_code                         AS code,
    reason_display                      AS display
FROM encounter_reason;

-- ── Statement 5: Status History (level 3 hierarchy example) ───────────────
-- encounter_status_history.csv kolommen: encounter_id, history_id, status, period_start, period_end
-- Meerdere rijen per encounter_id → array in Encounter.statusHistory
-- status kolom bevat broncode (bijv. "DONE") die via conceptmap wordt vertaald
-- naar een geldig FHIR EncounterStatus (bijv. "finished")
SELECT
    encounter_id                        AS resource_id,
    history_id                          AS id,
    encounter_id                        AS parent_id,
    'Encounter.statusHistory'           AS fhir_path,
    status,
    period_start                        AS "period.start",
    period_end                          AS "period.end"
FROM encounter_status_history;
