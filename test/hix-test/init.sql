-- HIX test database initialization
-- Table and column names match exactly what the query compiler SQL files expect.
-- Run automatically by setup.sh on container startup.

IF EXISTS (SELECT * FROM sys.databases WHERE name = 'hix')
    DROP DATABASE hix;
GO

CREATE DATABASE hix;
GO

USE hix;
GO

-- ─────────────────────────────────────────────────────────────────────────────
-- Patient
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE patients (
    patient_id  VARCHAR(20) PRIMARY KEY,
    gender      VARCHAR(20),
    birth_date  DATE,
    active      BIT
);

CREATE TABLE patient_names (
    name_id     VARCHAR(20) PRIMARY KEY,
    patient_id  VARCHAR(20) NOT NULL REFERENCES patients(patient_id),
    name_use    VARCHAR(20),
    family      VARCHAR(100),
    given       VARCHAR(100)
);

CREATE TABLE patient_telecom (
    telecom_id      VARCHAR(20) PRIMARY KEY,
    patient_id      VARCHAR(20) NOT NULL REFERENCES patients(patient_id),
    telecom_system  VARCHAR(20),
    telecom_value   VARCHAR(100),
    telecom_use     VARCHAR(20)
);

CREATE TABLE patient_address (
    address_id   VARCHAR(20) PRIMARY KEY,
    patient_id   VARCHAR(20) NOT NULL REFERENCES patients(patient_id),
    address_use  VARCHAR(20),
    line         VARCHAR(200),
    city         VARCHAR(100),
    postal_code  VARCHAR(10),
    country      VARCHAR(50)
);

CREATE TABLE patient_identifier (
    identifier_id  VARCHAR(20) PRIMARY KEY,
    patient_id     VARCHAR(20) NOT NULL REFERENCES patients(patient_id),
    id_use         VARCHAR(20),
    id_system      VARCHAR(200),
    id_value       VARCHAR(100)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Observation (general — obs_type != 'LAB' and != 'VITAL')
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE hix_observations (
    observation_id  VARCHAR(20) PRIMARY KEY,
    patient_id      VARCHAR(20) NOT NULL REFERENCES patients(patient_id),
    obs_status      VARCHAR(20),
    obs_date        DATE,
    obs_value       VARCHAR(100),
    obs_unit        VARCHAR(50),
    obs_type        VARCHAR(20),  -- 'GENERAL' | 'LAB' | 'VITAL'
    updated_at      DATETIME DEFAULT GETDATE()
);

CREATE TABLE hix_obs_category (
    cat_id          VARCHAR(20) PRIMARY KEY,
    observation_id  VARCHAR(20) NOT NULL REFERENCES hix_observations(observation_id),
    cat_text        VARCHAR(100)
);

CREATE TABLE hix_obs_category_coding (
    coding_id       VARCHAR(20) PRIMARY KEY,
    cat_id          VARCHAR(20) NOT NULL REFERENCES hix_obs_category(cat_id),
    coding_system   VARCHAR(200),
    coding_code     VARCHAR(50),
    coding_display  VARCHAR(200)
);

CREATE TABLE hix_obs_coding (
    coding_id       VARCHAR(20) PRIMARY KEY,
    observation_id  VARCHAR(20) NOT NULL REFERENCES hix_observations(observation_id),
    coding_system   VARCHAR(200),
    coding_code     VARCHAR(50),
    coding_display  VARCHAR(200)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Lab results (separate table, obs_type = 'LAB')
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE hix_lab_results (
    lab_id          VARCHAR(20) PRIMARY KEY,
    patient_id      VARCHAR(20) NOT NULL REFERENCES patients(patient_id),
    lab_status      VARCHAR(20),
    result_date     DATE,
    result_value    VARCHAR(100),
    result_unit     VARCHAR(50),
    loinc_code      VARCHAR(20),
    loinc_display   VARCHAR(200),
    updated_at      DATETIME DEFAULT GETDATE()
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Vital signs (separate table, obs_type = 'VITAL')
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE hix_vitals (
    vital_id        VARCHAR(20) PRIMARY KEY,
    patient_id      VARCHAR(20) NOT NULL REFERENCES patients(patient_id),
    vital_status    VARCHAR(20),
    measured_at     DATETIME,
    vital_value     VARCHAR(100),
    vital_unit      VARCHAR(50),
    loinc_code      VARCHAR(20),
    loinc_display   VARCHAR(200),
    updated_at      DATETIME DEFAULT GETDATE()
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Encounter
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE encounters (
    encounter_id      VARCHAR(20) PRIMARY KEY,
    patient_id        VARCHAR(20) NOT NULL REFERENCES patients(patient_id),
    status            VARCHAR(20),
    start_time        DATETIME,
    end_time          DATETIME,
    service_provider  VARCHAR(200)
);

CREATE TABLE encounter_type (
    type_id       VARCHAR(20) PRIMARY KEY,
    encounter_id  VARCHAR(20) NOT NULL REFERENCES encounters(encounter_id),
    type_text     VARCHAR(200)
);

CREATE TABLE encounter_type_coding (
    coding_id       VARCHAR(20) PRIMARY KEY,
    type_id         VARCHAR(20) NOT NULL REFERENCES encounter_type(type_id),
    coding_system   VARCHAR(200),
    coding_code     VARCHAR(50),
    coding_display  VARCHAR(200)
);

CREATE TABLE encounter_reason (
    reason_id       VARCHAR(20) PRIMARY KEY,
    encounter_id    VARCHAR(20) NOT NULL REFERENCES encounters(encounter_id),
    reason_system   VARCHAR(200),
    reason_code     VARCHAR(50),
    reason_display  VARCHAR(200)
);

CREATE TABLE encounter_status_history (
    history_id    VARCHAR(20) PRIMARY KEY,
    encounter_id  VARCHAR(20) NOT NULL REFERENCES encounters(encounter_id),
    status        VARCHAR(20),
    period_start  DATETIME,
    period_end    DATETIME
);
GO

-- ─────────────────────────────────────────────────────────────────────────────
-- Test data
-- ─────────────────────────────────────────────────────────────────────────────

INSERT INTO patients (patient_id, gender, birth_date, active) VALUES
('P001', 'male',   '1975-03-15', 1),
('P002', 'female', '1982-07-22', 1),
('P003', 'male',   '1968-11-05', 1),
('P004', 'female', '1990-01-30', 1),
('P005', 'other',  '1985-06-18', 1);

INSERT INTO patient_names (name_id, patient_id, name_use, family, given) VALUES
('N001', 'P001', 'official', 'Jansen',   'Jan'),
('N002', 'P001', 'maiden',   'Pietersen','Jan'),
('N003', 'P002', 'official', 'De Vries', 'Anna'),
('N004', 'P003', 'official', 'Bakker',   'Pieter'),
('N005', 'P004', 'official', 'Mooi',     'Sonja'),
('N006', 'P005', 'official', 'Hendrix',  'Alex');

INSERT INTO patient_telecom (telecom_id, patient_id, telecom_system, telecom_value, telecom_use) VALUES
('T001', 'P001', 'phone', '+31612345678',           'mobile'),
('T002', 'P001', 'email', 'jan.jansen@example.com', 'home'),
('T003', 'P002', 'phone', '+31687654321',           'mobile'),
('T004', 'P002', 'email', 'anna.vries@example.com', 'work'),
('T005', 'P003', 'phone', '+31611223344',           'home'),
('T006', 'P004', 'phone', '+31655667788',           'mobile'),
('T007', 'P005', 'email', 'alex.hendrix@example.com','home');

INSERT INTO patient_address (address_id, patient_id, address_use, line, city, postal_code, country) VALUES
('A001', 'P001', 'home', 'Hoofdstraat 123',     'Amsterdam', '1012 AB', 'NL'),
('A002', 'P001', 'work', 'Bedrijfslaan 45',     'Amsterdam', '1082 PR', 'NL'),
('A003', 'P002', 'home', 'Grachtengordel 789',  'Amsterdam', '1015 KH', 'NL'),
('A004', 'P003', 'home', 'Museumplein 1',       'Amsterdam', '1071 XA', 'NL'),
('A005', 'P004', 'home', 'Westerstraat 321',    'Amsterdam', '1015 MV', 'NL'),
('A006', 'P005', 'home', 'Jordaan 654',         'Amsterdam', '1015 MZ', 'NL');

INSERT INTO patient_identifier (identifier_id, patient_id, id_use, id_system, id_value) VALUES
('ID001', 'P001', 'usual',    'http://example.com/mrn', 'MR12345678'),
('ID002', 'P001', 'official', 'http://fhir.nl/fhir/NamingSystem/bsn', '123456789'),
('ID003', 'P002', 'usual',    'http://example.com/mrn', 'MR87654321'),
('ID004', 'P002', 'official', 'http://fhir.nl/fhir/NamingSystem/bsn', '987654321'),
('ID005', 'P003', 'usual',    'http://example.com/mrn', 'MR11223344'),
('ID006', 'P003', 'official', 'http://fhir.nl/fhir/NamingSystem/bsn', '456789012'),
('ID007', 'P004', 'usual',    'http://example.com/mrn', 'MR55667788'),
('ID008', 'P004', 'official', 'http://fhir.nl/fhir/NamingSystem/bsn', '789012345'),
('ID009', 'P005', 'usual',    'http://example.com/mrn', 'MR99887766'),
('ID010', 'P005', 'official', 'http://fhir.nl/fhir/NamingSystem/bsn', '234567890');

-- General observations
INSERT INTO hix_observations (observation_id, patient_id, obs_status, obs_date, obs_value, obs_unit, obs_type) VALUES
('OBS001', 'P001', 'final',     '2023-02-10', '120/80',  'mmHg',  'GENERAL'),
('OBS002', 'P002', 'final',     '2023-05-14', '98.6',    'F',     'GENERAL'),
('OBS003', 'P003', 'amended',   '2022-11-20', '72',      'bpm',   'GENERAL'),
('OBS004', 'P004', 'final',     '2024-01-08', '36.7',    'C',     'GENERAL');

INSERT INTO hix_obs_category (cat_id, observation_id, cat_text) VALUES
('CAT001', 'OBS001', 'Vital Signs'),
('CAT002', 'OBS002', 'Vital Signs'),
('CAT003', 'OBS003', 'Vital Signs'),
('CAT004', 'OBS004', 'Vital Signs');

INSERT INTO hix_obs_category_coding (coding_id, cat_id, coding_system, coding_code, coding_display) VALUES
('CC001', 'CAT001', 'http://terminology.hl7.org/CodeSystem/observation-category', 'vital-signs', 'Vital Signs'),
('CC002', 'CAT002', 'http://terminology.hl7.org/CodeSystem/observation-category', 'vital-signs', 'Vital Signs'),
('CC003', 'CAT003', 'http://terminology.hl7.org/CodeSystem/observation-category', 'vital-signs', 'Vital Signs'),
('CC004', 'CAT004', 'http://terminology.hl7.org/CodeSystem/observation-category', 'vital-signs', 'Vital Signs');

INSERT INTO hix_obs_coding (coding_id, observation_id, coding_system, coding_code, coding_display) VALUES
('OC001', 'OBS001', 'http://loinc.org', '55284-4', 'Blood pressure systolic and diastolic'),
('OC002', 'OBS002', 'http://loinc.org', '8310-5',  'Body temperature'),
('OC003', 'OBS003', 'http://loinc.org', '8867-4',  'Heart rate'),
('OC004', 'OBS004', 'http://loinc.org', '8310-5',  'Body temperature');

-- Lab results
INSERT INTO hix_lab_results (lab_id, patient_id, lab_status, result_date, result_value, result_unit, loinc_code, loinc_display) VALUES
('LAB001', 'P001', 'final',  '2023-03-01', '5.2',  'mmol/L',  '2339-0', 'Glucose [Mass/volume] in Blood'),
('LAB002', 'P002', 'final',  '2023-06-15', '4.8',  'mmol/L',  '2339-0', 'Glucose [Mass/volume] in Blood'),
('LAB003', 'P003', 'final',  '2022-12-05', '140',  'g/L',     '718-7',  'Hemoglobin [Mass/volume] in Blood'),
('LAB004', 'P004', 'final',  '2024-02-20', '7.1',  'mmol/L',  '2339-0', 'Glucose [Mass/volume] in Blood'),
('LAB005', 'P001', 'final',  '2023-09-10', '138',  'g/L',     '718-7',  'Hemoglobin [Mass/volume] in Blood');

-- Vital signs
INSERT INTO hix_vitals (vital_id, patient_id, vital_status, measured_at, vital_value, vital_unit, loinc_code, loinc_display) VALUES
('VIT001', 'P001', 'final', '2023-02-10 09:30:00', '78',  'kg',   '29463-7', 'Body weight'),
('VIT002', 'P002', 'final', '2023-05-14 14:15:00', '65',  'kg',   '29463-7', 'Body weight'),
('VIT003', 'P003', 'final', '2022-11-20 11:00:00', '182', 'cm',   '8302-2',  'Body height'),
('VIT004', 'P004', 'final', '2024-01-08 10:45:00', '62',  'kg',   '29463-7', 'Body weight'),
('VIT005', 'P005', 'final', '2023-08-22 16:00:00', '98',  '%',    '2708-6',  'Oxygen saturation in Arterial blood');

-- Encounters
INSERT INTO encounters (encounter_id, patient_id, status, start_time, end_time, service_provider) VALUES
('ENC001', 'P001', 'finished',   '2023-02-10 09:00:00', '2023-02-10 10:00:00', 'Cardiology'),
('ENC002', 'P002', 'finished',   '2023-05-14 14:00:00', '2023-05-14 15:00:00', 'General Practice'),
('ENC003', 'P003', 'finished',   '2022-11-20 10:30:00', '2022-11-21 11:00:00', 'Internal Medicine'),
('ENC004', 'P004', 'in-progress','2024-01-08 10:00:00', NULL,                  'Obstetrics'),
('ENC005', 'P001', 'finished',   '2023-09-05 08:00:00', '2023-09-05 09:30:00', 'Radiology');

INSERT INTO encounter_type (type_id, encounter_id, type_text) VALUES
('ET001', 'ENC001', 'Outpatient visit'),
('ET002', 'ENC002', 'Outpatient visit'),
('ET003', 'ENC003', 'Inpatient admission'),
('ET004', 'ENC004', 'Outpatient visit'),
('ET005', 'ENC005', 'Outpatient visit');

INSERT INTO encounter_type_coding (coding_id, type_id, coding_system, coding_code, coding_display) VALUES
('ETC001', 'ET001', 'http://snomed.info/sct', '11429006', 'Consultation'),
('ETC002', 'ET002', 'http://snomed.info/sct', '11429006', 'Consultation'),
('ETC003', 'ET003', 'http://snomed.info/sct', '32485007', 'Hospital admission'),
('ETC004', 'ET004', 'http://snomed.info/sct', '11429006', 'Consultation'),
('ETC005', 'ET005', 'http://snomed.info/sct', '11429006', 'Consultation');

INSERT INTO encounter_reason (reason_id, encounter_id, reason_system, reason_code, reason_display) VALUES
('ER001', 'ENC001', 'http://snomed.info/sct', '38341003',  'Hypertension'),
('ER002', 'ENC002', 'http://snomed.info/sct', '44054006',  'Type 2 diabetes mellitus'),
('ER003', 'ENC003', 'http://snomed.info/sct', '57054005',  'Acute myocardial infarction'),
('ER004', 'ENC004', 'http://snomed.info/sct', '72892002',  'Normal pregnancy'),
('ER005', 'ENC005', 'http://snomed.info/sct', '169069000', 'Imaging procedure');

INSERT INTO encounter_status_history (history_id, encounter_id, status, period_start, period_end) VALUES
('ESH001', 'ENC001', 'arrived',  '2023-02-10 09:00:00', '2023-02-10 09:10:00'),
('ESH002', 'ENC001', 'finished', '2023-02-10 09:10:00', '2023-02-10 10:00:00'),
('ESH003', 'ENC003', 'arrived',  '2022-11-20 10:30:00', '2022-11-20 11:00:00'),
('ESH004', 'ENC003', 'finished', '2022-11-20 11:00:00', '2022-11-21 11:00:00'),
('ESH005', 'ENC004', 'arrived',  '2024-01-08 10:00:00', NULL);
GO
