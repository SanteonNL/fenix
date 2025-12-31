-- SQL Server compatible setup script

CREATE TABLE patient (
    identificatienummer VARCHAR(13),
    geslachtcode VARCHAR(255),
    geslachtomschrijving VARCHAR(255),
    gerelateerdpersoonid VARCHAR(20),
    gerelateerderelatie VARCHAR(255),
    land VARCHAR(100),
    geboortedatum DATETIME2,
    datumoverlijden DATE,
    datumcheckstatusoverlijden VARCHAR(255)
);

INSERT INTO patient (identificatienummer, geslachtcode, geslachtomschrijving, gerelateerdpersoonid, gerelateerderelatie, land, geboortedatum, datumoverlijden, datumcheckstatusoverlijden)
VALUES ('123', 'M', 'Male', '987654321', '123456', 'USA', '1990-01-01', '2022-05-10', 'Checked');

INSERT INTO patient (identificatienummer, geslachtcode, geslachtomschrijving, gerelateerdpersoonid, gerelateerderelatie, land, geboortedatum, datumoverlijden, datumcheckstatusoverlijden)
VALUES ('987', 'F', 'Female', '123456789', '654321', 'UK', '1985-05-20', NULL, 'Unchecked');

INSERT INTO patient (identificatienummer, geslachtcode, geslachtomschrijving, gerelateerdpersoonid, gerelateerderelatie, land, geboortedatum, datumoverlijden, datumcheckstatusoverlijden)
VALUES ('456', 'M', 'Male', '789012345', '789', 'Canada', '1978-12-10', NULL, 'Unchecked');

INSERT INTO patient (identificatienummer, geslachtcode, geslachtomschrijving, gerelateerdpersoonid, gerelateerderelatie, land, geboortedatum, datumoverlijden, datumcheckstatusoverlijden)
VALUES ('789', 'F', 'Female', '456789012', '567890', 'Australia', '1995-08-15', NULL, 'Unchecked');

INSERT INTO patient (identificatienummer, geslachtcode, geslachtomschrijving, gerelateerdpersoonid, gerelateerderelatie, land, geboortedatum, datumoverlijden, datumcheckstatusoverlijden)
VALUES ('234', 'M', 'Male', '890123456', '901234', 'Germany', '1980-03-25', '2021-11-30', 'Checked');

CREATE TABLE names (
    identificatienummer VARCHAR(13),
    firstname VARCHAR(255),
    lastname VARCHAR(255),
    name_use VARCHAR(255),
    period_start DATE,
    period_end DATE
);

INSERT INTO names (identificatienummer, firstname, lastname, name_use, period_start, period_end)
VALUES ('123', 'John', 'Doe', 'Official', '2022-01-01', '2022-12-31');

INSERT INTO names (identificatienummer, firstname, lastname, name_use, period_start, period_end)
VALUES ('987', 'Alice', 'Johnson', 'Official', '2022-01-01', '2022-12-31');

INSERT INTO names (identificatienummer, firstname, lastname, name_use, period_start, period_end)
VALUES ('987', 'Bob', 'Williams', 'Alternate', '2022-01-01', '2022-12-31');

INSERT INTO names (identificatienummer, firstname, lastname, name_use, period_start, period_end)
VALUES ('456', 'Michael', 'Brown', 'Official', '2022-01-01', '2022-12-31');

INSERT INTO names (identificatienummer, firstname, lastname, name_use, period_start, period_end)
VALUES ('456', 'Emily', 'Davis', 'Alternate', '2022-01-01', '2022-12-31');

INSERT INTO names (identificatienummer, firstname, lastname, name_use, period_start, period_end)
VALUES ('789', 'David', 'Miller', 'Official', '2022-01-01', '2022-12-31');

INSERT INTO names (identificatienummer, firstname, lastname, name_use, period_start, period_end)
VALUES ('P002', 'Olivia', 'Wilson', 'Alternate', '2022-01-01', '2022-12-31');

INSERT INTO names (identificatienummer, firstname, lastname, name_use, period_start, period_end)
VALUES ('P001', 'Daniel', 'Anderson', 'Official', '2022-01-01', '2022-12-31');

INSERT INTO names (identificatienummer, firstname, lastname, name_use, period_start, period_end)
VALUES ('P001', 'Sophia', 'Taylor', 'Alternate', '2022-01-01', '2022-12-31');

CREATE TABLE practitioner (
    practitioner_id VARCHAR(20),
    practitioner_name VARCHAR(255)
);

INSERT INTO practitioner (practitioner_id, practitioner_name)
VALUES ('P001', 'Dr. Smith');

INSERT INTO practitioner (practitioner_id, practitioner_name)
VALUES ('P002', 'Dr. Johnson');

CREATE TABLE patient_practitioner (
    identificatienummer VARCHAR(13),
    practitioner_id VARCHAR(20)
);

INSERT INTO patient_practitioner (identificatienummer, practitioner_id)
VALUES (RIGHT('123', 3), 'P001');

INSERT INTO patient_practitioner (identificatienummer, practitioner_id)
VALUES (RIGHT('987', 3), 'P001a');

INSERT INTO patient_practitioner (identificatienummer, practitioner_id)
VALUES (RIGHT('456', 3), 'P002');

INSERT INTO patient_practitioner (identificatienummer, practitioner_id)
VALUES (RIGHT('789', 3), 'P002');

INSERT INTO patient_practitioner (identificatienummer, practitioner_id)
VALUES (RIGHT('234', 3), 'P002');


INSERT INTO names (identificatienummer, firstname, lastname, name_use)
VALUES (RIGHT('123', 3), 'Mark', 'Johnson', 'Official');

INSERT INTO names (identificatienummer, firstname, lastname, name_use)
VALUES (RIGHT('123', 3), 'Sarah', 'Smith', 'Alternate');

INSERT INTO names (identificatienummer, firstname, lastname, name_use)
VALUES (RIGHT('987', 3), 'Emily', 'Brown', 'Official');

INSERT INTO names (identificatienummer, firstname, lastname, name_use)
VALUES (RIGHT('987', 3), 'Jacob', 'Davis', 'Alternate');

INSERT INTO names (identificatienummer, firstname, lastname, name_use)
VALUES (RIGHT('456', 3), 'Emma', 'Miller', 'Official');

INSERT INTO names (identificatienummer, firstname, lastname, name_use)
VALUES (RIGHT('456', 3), 'Noah', 'Wilson', 'Alternate');

INSERT INTO names (identificatienummer, firstname, lastname, name_use)
VALUES (RIGHT('789', 3), 'Liam', 'Anderson', 'Official');

INSERT INTO names (identificatienummer, firstname, lastname, name_use)
VALUES (RIGHT('789', 3), 'Ava', 'Taylor', 'Alternate');

INSERT INTO names (identificatienummer, firstname, lastname, name_use)
VALUES (RIGHT('234', 3), 'Mia', 'Anderson', 'Official');

INSERT INTO names (identificatienummer, firstname, lastname, name_use)
VALUES (RIGHT('234', 3), 'Ethan', 'Taylor', 'Alternate');

-- Create new mothers for existing patients
INSERT INTO patient (identificatienummer, geslachtcode, geslachtomschrijving, gerelateerdpersoonid, gerelateerderelatie, land, geboortedatum, datumoverlijden, datumcheckstatusoverlijden)
VALUES ('123M', 'F', 'Female', '123', 'Mother', 'USA', '1970-01-01', NULL, 'Unchecked');

INSERT INTO patient (identificatienummer, geslachtcode, geslachtomschrijving, gerelateerdpersoonid, gerelateerderelatie, land, geboortedatum, datumoverlijden, datumcheckstatusoverlijden)
VALUES ('987M', 'F', 'Female', '987', 'Mother', 'UK', '1965-05-20', NULL, 'Unchecked');

INSERT INTO patient (identificatienummer, geslachtcode, geslachtomschrijving, gerelateerdpersoonid, gerelateerderelatie, land, geboortedatum, datumoverlijden, datumcheckstatusoverlijden)
VALUES ('456M', 'F', 'Female', '456', 'Mother', 'Canada', '1958-12-10', NULL, 'Unchecked');

INSERT INTO patient (identificatienummer, geslachtcode, geslachtomschrijving, gerelateerdpersoonid, gerelateerderelatie, land, geboortedatum, datumoverlijden, datumcheckstatusoverlijden)
VALUES ('789M', 'F', 'Female', '789', 'Mother', 'Australia', '1985-08-15', NULL, 'Unchecked');

INSERT INTO patient (identificatienummer, geslachtcode, geslachtomschrijving, gerelateerdpersoonid, gerelateerderelatie, land, geboortedatum, datumoverlijden, datumcheckstatusoverlijden)
VALUES ('234M', 'F', 'Female', '234', 'Mother', 'Germany', '1960-03-25', NULL, 'Unchecked');

-- Create a new couple table that relates a patientid to its mother
CREATE TABLE couple (
    patient_id VARCHAR(13),
    mother_id VARCHAR(13)
);

INSERT INTO couple (patient_id, mother_id)
VALUES ('123', '123M');

INSERT INTO couple (patient_id, mother_id)
VALUES ('987', '987M');

INSERT INTO couple (patient_id, mother_id)
VALUES ('456', '456M');

INSERT INTO couple (patient_id, mother_id)
VALUES ('789', '789M');

INSERT INTO couple (patient_id, mother_id)
VALUES ('234', '234M');


CREATE TABLE contacts (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100),
    relationship VARCHAR(50),
    gender VARCHAR(10),
    organization VARCHAR(100),
    patient_id VARCHAR(50)
);

INSERT INTO contacts (id, name, relationship, gender, organization, patient_id)
VALUES ('456', 'John Doe', 'Friend', 'male', 'Hospital A', '123');

INSERT INTO contacts (id, name, relationship, gender, organization, patient_id)
VALUES ('789', 'Jane Smith', 'Family', 'female', 'Hospital B', '123');


CREATE TABLE contact_points (
    id INT IDENTITY(1,1) PRIMARY KEY,
    contact_id VARCHAR(50),
    [system] VARCHAR(50),
    [value] VARCHAR(100),
    [use] VARCHAR(50),
    FOREIGN KEY (contact_id) REFERENCES contacts(id)
);

INSERT INTO contact_points (contact_id, [system], [value], [use])
VALUES ('456', 'phone', '+1234567890', 'home');

INSERT INTO contact_points (contact_id, [system], [value], [use])
VALUES ('456', 'email', 'john.doe@example.com', 'work');

INSERT INTO contact_points (contact_id, [system], [value], [use])
VALUES ('789', 'phone', '+9876543210', 'mobile');

-- Modify the table structure
CREATE TABLE observation_raw (
    metingid VARCHAR(18) PRIMARY KEY,
    identificatienummer VARCHAR(13),
    metingnaamcodesysteem VARCHAR(255),
    metingnaamcode VARCHAR(255),
    metingnaamomschrijving VARCHAR(255),
    metingdatumtijd DATETIME2,
    meetmethodecodesysteem VARCHAR(255),
    meetmethodecode VARCHAR(255),
    uitslagwaarde VARCHAR(50),  -- Changed to VARCHAR to accommodate non-numeric values
    uitslagwaardeeenheid VARCHAR(50),
    uitslagwaardeoperator VARCHAR(1),
    uitslagcodesysteem VARCHAR(255),
    uitslagcode VARCHAR(255),
    uitslagcodeomschrijving VARCHAR(255),
    uitslagdatumtijd DATETIME2
);

-- Insert sample data
INSERT INTO observation_raw (
    metingid, identificatienummer, metingnaamcodesysteem, metingnaamcode, metingnaamomschrijving,
    metingdatumtijd, meetmethodecodesysteem, meetmethodecode, uitslagwaarde, uitslagwaardeeenheid,
    uitslagwaardeoperator, uitslagcodesysteem, uitslagcode, uitslagcodeomschrijving, uitslagdatumtijd
) VALUES
-- Examples from the CSV data
('1', '456', NULL, 'foutecode', NULL, '2021-10-30T19:19:19', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL),
('2', '456', 'http://loinc.org', '97816-3', NULL, '2021-10-30T19:19:19', NULL, NULL, NULL, NULL, NULL, NULL, 'LA32131-7', NULL, NULL),
('3', '456', NULL, 'ASA', NULL, '2022-10-30T19:19:19', NULL, NULL, NULL, NULL, NULL, NULL, 'ASA-4', NULL, NULL),
('4', '6121', 'http://loinc.org', 'ASA3', NULL, '2021-10-30T19:19:19', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL),
('5', '6121', 'http://loinc.org', 'ASA900', NULL, '2021-10-30T19:19:19', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL),
('6', '6121', 'http://loinc.org', 'ASA900', NULL, '2021-10-30T19:19:19', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL),
('7', '456', 'http://snomed.info/sct', '276477006', NULL, '2022-10-30T19:19:19', NULL, NULL, NULL, NULL, NULL, NULL, '22636003', NULL, NULL),
('8', '456', NULL, '276477006', NULL, '2022-10-30T19:19:19', NULL, NULL, NULL, NULL, NULL, NULL, '22636003', NULL, NULL),
('9', '456', 'http://snomed.info/sct', '276477006', NULL, '2022-10-30T19:19:19', NULL, NULL, NULL, NULL, NULL, NULL, 'UNK', NULL, NULL),
('10', '456', 'http://snomed.info/sct', '276477006', NULL, '2022-10-30T19:19:19', NULL, NULL, NULL, NULL, NULL, 'http://terminology.hl7.org/CodeSystem/v3-AcknowledgementCondition', 'UNK', NULL, NULL),
('11', '456', 'http://snomed.info/sct', '276477006', NULL, '2022-10-30T19:19:19', NULL, NULL, 'UNK', NULL, NULL, 'http://terminology.hl7.org/CodeSystem/v3-AcknowledgementCondition', NULL, NULL, NULL),
('12', '456', 'http://snomed.info/sct', '276477006', NULL, '2022-10-30T19:19:19', NULL, NULL, '22636003', NULL, NULL, 'http://terminology.hl7.org/CodeSystem/v3-AcknowledgementCondition', NULL, NULL, NULL),
('13', '456', 'urn:oid:1.2.840.114350.1.13.222.2.7.2.727688', 'SAZ#6594', NULL, '2022-10-30T19:19:19', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL),
('14', '456', 'urn:oid:1.2.840.114350.1.13.222.2.7.2.727688', 'SAZ#6594', NULL, '2022-10-30T19:19:19', NULL, NULL, '015', NULL, NULL, NULL, NULL, NULL, NULL),
('15', '456', 'urn:oid:1.2.840.114350.1.13.222.2.7.2.727688', 'SAZ#6594', NULL, '2022-10-30T19:19:19', NULL, NULL, '015', NULL, NULL, NULL, '', NULL, NULL),

-- Additional examples with various data patterns
('16', '789', 'http://loinc.org', '8480-6', 'Systolic blood pressure', '2023-06-15T10:30:00', NULL, NULL, '120', 'mm[Hg]', NULL, NULL, NULL, NULL, NULL),
('17', '789', 'http://loinc.org', '8462-4', 'Diastolic blood pressure', '2023-06-15T10:30:00', NULL, NULL, '80', 'mm[Hg]', NULL, NULL, NULL, NULL, NULL),
('18', '101', 'http://snomed.info/sct', '167271000', 'Glucose measurement', '2023-06-16T08:15:00', NULL, NULL, '5.4', 'mmol/L', NULL, NULL, NULL, NULL, NULL),
('19', '101', 'http://snomed.info/sct', '27113001', 'Body weight', '2023-06-16T08:20:00', NULL, NULL, '70.5', 'kg', NULL, NULL, NULL, NULL, NULL),
('20', '202', 'http://loinc.org', '8310-5', 'Body temperature', '2023-06-17T14:45:00', NULL, NULL, '37.2', 'Cel', NULL, NULL, NULL, NULL, NULL),
('21', '456', 'http://loinc.org', '8867-4', 'Heart rate', '2023-06-17T14:45:00', NULL, NULL, '72', '/min', NULL, NULL, NULL, NULL, NULL),
('22', '456', 'http://snomed.info/sct', '444814009', 'Viral load', '2023-06-18T11:30:00', NULL, NULL, '50', 'copies/mL', '<', NULL, NULL, 'Below detectable limit', NULL),
('23', '456', 'http://loinc.org', '2339-0', 'Glucose tolerance test', '2023-06-18T12:00:00', NULL, NULL, '11.1', 'mmol/L', '>', NULL, NULL, 'Abnormal', NULL),
('24', '456', 'http://snomed.info/sct', '365853002', 'Hemoglobin level', '2023-06-19T09:00:00', NULL, NULL, '14.5', 'g/dL', NULL, NULL, NULL, NULL, NULL);

-- Modify the table structure for Encounter
CREATE TABLE encounter_raw (
    encounter_id VARCHAR(18) PRIMARY KEY,
    identificatienummer VARCHAR(13),
    encounter_type_codesystem VARCHAR(255),
    encounter_type_code VARCHAR(255),
    encounter_type_description VARCHAR(255),
    encounter_start_time DATETIME2,
    encounter_end_time DATETIME2,
    service_provider VARCHAR(255),
    encounter_status VARCHAR(50),
    class_codesystem VARCHAR(255),
    class_code VARCHAR(255),
    class_description VARCHAR(255),
    reason_codesystem VARCHAR(255),
    reason_code VARCHAR(255),
    reason_description VARCHAR(255),
    admission_timestamp DATETIME2,
    discharge_timestamp DATETIME2
);

-- Insert sample data
INSERT INTO encounter_raw (
    encounter_id, identificatienummer, encounter_type_codesystem, encounter_type_code, encounter_type_description,
    encounter_start_time, encounter_end_time, service_provider, encounter_status, class_codesystem,
    class_code, class_description, reason_codesystem, reason_code, reason_description,
    admission_timestamp, discharge_timestamp
) VALUES
    ('ENC001', '456', 'SystemA', 'E001', 'Emergency Visit', '2023-10-15 08:00:00',
    '2023-10-15 12:00:00', 'General Hospital', 'completed', 'ClassSystem1', 'ER', 'Emergency Room',
    'ReasonSystem1', 'R01', 'Acute Chest Pain', '2023-10-15 08:00:00', '2023-10-15 12:00:00'),

    ('ENC002', '456', 'SystemB', 'I001', 'Inpatient Visit', '2023-10-16 09:00:00',
    '2023-10-20 15:00:00', 'City Clinic', 'in-progress', 'ClassSystem2', 'INP', 'Inpatient',
    'ReasonSystem2', 'R02', 'Post-Surgical Recovery', '2023-10-16 09:00:00', NULL);

-- Create the questionnaire_raw table
CREATE TABLE questionnaire_raw (
    identificatienummer VARCHAR(13), -- "subject.reference" (e.g., Patient/456)
    questionnaire_id VARCHAR(18) PRIMARY KEY, -- "id"
    code_codesystem VARCHAR(255), -- "code.coding[0].system"
    code_code VARCHAR(255), -- "code.coding[0].code"
    code_display VARCHAR(255), -- "code.coding[0].display"
    status VARCHAR(50), -- "status"
    date DATETIME2, -- "date"
    item_linkId VARCHAR(255), -- "item[n].linkId" (where n is the item index)
    item_text VARCHAR(255), -- "item[n].text"
    item_type VARCHAR(50), -- "item[n].type"
    item_code_codesystem VARCHAR(255), -- "item[n].code.coding[0].system"
    item_code_code VARCHAR(255), -- "item[n].code.coding[0].code"
    item_code_display VARCHAR(255) -- "item[n].code.coding[0].display"
);

-- Insert sample data
INSERT INTO questionnaire_raw (
    questionnaire_id, identificatienummer, code_codesystem, code_code,
    code_display, status, date, item_linkId,
    item_text, item_type,
    item_code_codesystem, item_code_code, item_code_display
) VALUES
    ('QST001', '456', 'http://terminology.hl7.org/CodeSystem/questionnaire-type', 'Q001',
    'Patient Satisfaction Survey', 'active', '2023-10-15 08:00:00',
    'ITEM01', 'How satisfied are you?', 'string',
    'http://terminology.hl7.org/CodeSystem/question-response', 'Satisfaction', 'Satisfaction'
    ),

    ('QST002', '456', 'http://terminology.hl7.org/CodeSystem/questionnaire-type', 'Q002',
    'Health Risk Assessment', 'retired', '2023-10-16 09:00:00',
    'ITEM02', 'Do you smoke?', 'boolean',
    'http://terminology.hl7.org/CodeSystem/question-response', 'SmokingStatus', 'Smoking Status'
);
