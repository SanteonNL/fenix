-- T-SQL initialization script for HIX test database
-- Creates HIX-like tables with raw Dutch column names and sample data

-- Create database
IF EXISTS (SELECT * FROM sys.databases WHERE name = 'hix')
    DROP DATABASE hix;

CREATE DATABASE hix;
GO

USE hix;
GO

-- Patients table (raw HIX-like names)
CREATE TABLE Patients (
    PatientNummer VARCHAR(20) PRIMARY KEY,
    Geslacht VARCHAR(1),           -- M/V/O
    Geboortedatum DATE,
    Actief BIT
);

-- Patient Names
CREATE TABLE PatientNamen (
    NaamId VARCHAR(20) PRIMARY KEY,
    PatientNummer VARCHAR(20) NOT NULL,
    NaamGebruik VARCHAR(20),       -- official, nickname
    Achternaam VARCHAR(100),
    Voornaam VARCHAR(100),
    FOREIGN KEY (PatientNummer) REFERENCES Patients(PatientNummer)
);

-- Patient Telecom
CREATE TABLE PatientTelecom (
    TelecomId VARCHAR(20) PRIMARY KEY,
    PatientNummer VARCHAR(20) NOT NULL,
    TelecomSysteem VARCHAR(20),    -- phone, email
    TelecomWaarde VARCHAR(100),
    TelecomGebruik VARCHAR(20),    -- home, work, mobile
    FOREIGN KEY (PatientNummer) REFERENCES Patients(PatientNummer)
);

-- Patient Adressen (Addresses)
CREATE TABLE PatientAdressen (
    AdresId VARCHAR(20) PRIMARY KEY,
    PatientNummer VARCHAR(20) NOT NULL,
    AdresGebruik VARCHAR(20),      -- home, work
    Straat VARCHAR(200),
    Stad VARCHAR(100),
    Postcode VARCHAR(10),
    Land VARCHAR(50),
    FOREIGN KEY (PatientNummer) REFERENCES Patients(PatientNummer)
);

-- Patient Identificatie (Identifiers)
CREATE TABLE PatientIdentificatie (
    IdentificatieId VARCHAR(20) PRIMARY KEY,
    PatientNummer VARCHAR(20) NOT NULL,
    IdGebruik VARCHAR(20),
    IdSysteem VARCHAR(100),
    IdWaarde VARCHAR(100),
    FOREIGN KEY (PatientNummer) REFERENCES Patients(PatientNummer)
);

-- BSN Koppeling (BSN to HIX patient number mapping)
CREATE TABLE BSNKoppeling (
    BSN VARCHAR(9) PRIMARY KEY,
    PatientNummer VARCHAR(20) NOT NULL,
    FOREIGN KEY (PatientNummer) REFERENCES Patients(PatientNummer)
);

-- Insert test data

-- Patients
INSERT INTO Patients (PatientNummer, Geslacht, Geboortedatum, Actief) VALUES
('P001', 'M', '1975-03-15', 1),
('P002', 'V', '1982-07-22', 1),
('P003', 'M', '1968-11-05', 1),
('P004', 'V', '1990-01-30', 1),
('P005', 'O', '1985-06-18', 1);

-- Patient Names
INSERT INTO PatientNamen (NaamId, PatientNummer, NaamGebruik, Achternaam, Voornaam) VALUES
('N001', 'P001', 'official', 'Jansen', 'Jan'),
('N002', 'P001', 'maiden', 'Pietersen', 'Jan'),
('N003', 'P002', 'official', 'De Vries', 'Anna'),
('N004', 'P003', 'official', 'Bakker', 'Pieter'),
('N005', 'P004', 'official', 'Mooi', 'Sonja'),
('N006', 'P005', 'official', 'Hendrix', 'Alex');

-- Patient Telecom
INSERT INTO PatientTelecom (TelecomId, PatientNummer, TelecomSysteem, TelecomWaarde, TelecomGebruik) VALUES
('T001', 'P001', 'phone', '+31612345678', 'mobile'),
('T002', 'P001', 'email', 'jan.jansen@example.com', 'home'),
('T003', 'P002', 'phone', '+31687654321', 'mobile'),
('T004', 'P002', 'email', 'anna.vries@example.com', 'work'),
('T005', 'P003', 'phone', '+31611223344', 'home'),
('T006', 'P004', 'phone', '+31655667788', 'mobile'),
('T007', 'P005', 'email', 'alex.hendrix@example.com', 'home');

-- Patient Adressen
INSERT INTO PatientAdressen (AdresId, PatientNummer, AdresGebruik, Straat, Stad, Postcode, Land) VALUES
('A001', 'P001', 'home', 'Hoofdstraat 123', 'Amsterdam', '1012 AB', 'NL'),
('A002', 'P001', 'work', 'Bedrijfslaan 45', 'Amsterdam', '1082 PR', 'NL'),
('A003', 'P002', 'home', 'Grachtengordel 789', 'Amsterdam', '1015 KH', 'NL'),
('A004', 'P003', 'home', 'Museumplein 1', 'Amsterdam', '1071 XA', 'NL'),
('A005', 'P004', 'home', 'Westerstraat 321', 'Amsterdam', '1015 MV', 'NL'),
('A006', 'P005', 'home', 'Jordaan 654', 'Amsterdam', '1015 MZ', 'NL');

-- Patient Identificatie
INSERT INTO PatientIdentificatie (IdentificatieId, PatientNummer, IdGebruik, IdSysteem, IdWaarde) VALUES
('ID001', 'P001', 'usual', 'http://example.com/patient-identifier', 'MR12345678'),
('ID002', 'P001', 'official', 'http://example.com/bsn', '123456789'),
('ID003', 'P002', 'usual', 'http://example.com/patient-identifier', 'MR87654321'),
('ID004', 'P002', 'official', 'http://example.com/bsn', '987654321'),
('ID005', 'P003', 'usual', 'http://example.com/patient-identifier', 'MR11223344'),
('ID006', 'P003', 'official', 'http://example.com/bsn', '456789012'),
('ID007', 'P004', 'usual', 'http://example.com/patient-identifier', 'MR55667788'),
('ID008', 'P004', 'official', 'http://example.com/bsn', '789012345'),
('ID009', 'P005', 'usual', 'http://example.com/patient-identifier', 'MR99887766'),
('ID010', 'P005', 'official', 'http://example.com/bsn', '234567890');

-- BSN Koppeling (BSN to HIX patient number mapping)
INSERT INTO BSNKoppeling (BSN, PatientNummer) VALUES
('123456789', 'P001'),
('987654321', 'P002'),
('456789012', 'P003'),
('789012345', 'P004'),
('234567890', 'P005');

GO
