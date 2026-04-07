-- Patient SQL-to-FHIR query
-- Elk statement is een aparte resultset. De converter verwerkt ze na elkaar.
-- Meerdere rijen voor dezelfde fhir_path + parent_id worden automatisch een array.
--
-- Verwachte CSV-tabellen (geladen vanuit data/csv/):
--   patients.csv      → tabel "patients"
--   patient_names.csv → tabel "patient_names"
--   patient_telecom.csv → tabel "patient_telecom"
--   patient_address.csv → tabel "patient_address"
--   patient_identifier.csv → tabel "patient_identifier"
--
-- Verwachte kolommen per tabel: zie opmerkingen per statement.

-- ── Statement 1: Root Patient ──────────────────────────────────────────────
-- patients.csv kolommen: patient_id, gender, birth_date, active
SELECT
    patient_id      AS resource_id,
    patient_id      AS id,
    ''              AS parent_id,
    'Patient'       AS fhir_path,
    gender,
    birth_date      AS birthDate,
    active
FROM patients;

-- ── Statement 2: Naam ─────────────────────────────────────────────────────
-- patient_names.csv kolommen: patient_id, name_id, name_use, family, given
-- Meerdere rijen per patient_id → array in Patient.name
--
-- Voorbeeld CSV-inhoud:
--   patient_id, name_id, name_use, family, given
--   p001,       n1,      official, Jansen, Jan
--   p001,       n2,      maiden,   Pietersen, Jan
--   p002,       n3,      official, De Vries, Anna
SELECT
    patient_id                          AS resource_id,
    name_id                             AS id,
    patient_id                          AS parent_id,
    'Patient.name'                      AS fhir_path,
    name_use                            AS "use",
    family,
    given
FROM patient_names;

-- ── Statement 3: Telecom ──────────────────────────────────────────────────
-- patient_telecom.csv kolommen: patient_id, telecom_id, telecom_system, telecom_value, telecom_use
SELECT
    patient_id                          AS resource_id,
    telecom_id                          AS id,
    patient_id                          AS parent_id,
    'Patient.telecom'                   AS fhir_path,
    telecom_system                      AS "system",
    telecom_value                       AS "value",
    telecom_use                         AS "use"
FROM patient_telecom;

-- ── Statement 4: Adres ────────────────────────────────────────────────────
-- patient_address.csv kolommen: patient_id, address_id, address_use, line, city, postal_code, country
SELECT
    patient_id                          AS resource_id,
    address_id                          AS id,
    patient_id                          AS parent_id,
    'Patient.address'                   AS fhir_path,
    address_use                         AS "use",
    line,
    city,
    postal_code                         AS postalCode,
    country
FROM patient_address;

-- ── Statement 5: Identifier ───────────────────────────────────────────────
-- patient_identifier.csv kolommen: patient_id, identifier_id, id_use, id_system, id_value
SELECT
    patient_id                          AS resource_id,
    identifier_id                       AS id,
    patient_id                          AS parent_id,
    'Patient.identifier'                AS fhir_path,
    id_use                              AS "use",
    id_system                           AS "system",
    id_value                            AS "value"
FROM patient_identifier;
