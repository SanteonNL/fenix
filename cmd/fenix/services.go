package main

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"

	fhir "github.com/SanteonNL/fenix/models/fhir/r4"
	"github.com/SanteonNL/fenix/models/sim"
	"github.com/SanteonNL/fenix/util"
	"github.com/jinzhu/gorm"
)

type Config struct {
	Services []ServiceConfig `json:"services"`
}

// type PatientService interface {
// 	GetPatient(id string) (*fhir.Patient, error)
// 	GetAllPatients() ([]*fhir.Patient, error)
// }

type Application struct {
	Services []Service
}

type ServiceConfig struct {
	Type         string `json:"type"`
	Format       string `json:"format"`
	DatabaseType string `json:"databaseType"`
	ConnStr      string `json:"connStr"`
	SourcePath   string `json:"sourcePath"`
}

func NewService(config ServiceConfig) (Service, error) {
	switch config.Type {
	// case "postgres":
	// 	//return NewPostgreSQLService(config.ConnStr)
	// case "sqlserver":
	// 	//return NewSQLServerService(config.ConnStr)
	case "csv":
		switch config.Format {
		case "sim":
			return NewSIMCSVService(config.SourcePath), nil
		// case "fhir":
		// 	//return NewFHIRCSVService(config.FilePath), nil
		// case "castor":
		// 	// Create a Castor CSV service...
		default:
			return nil, fmt.Errorf("unsupported CSV format: %s", config.Format)
		}
	case "ndjson":
		switch config.Format {
		case "fhir":
			return NewFHIRNDJSONService(config.SourcePath), nil
		// case "castor":
		// 	// Create a Castor NDJSON service...
		default:
			return nil, fmt.Errorf("unsupported NDJSON format: %s", config.Format)
		}
	case "sql":
		switch config.DatabaseType {
		case "postgres":
			return NewSQLService(config.ConnStr, config.DatabaseType)
		// case "sqlserver":
		// 	//return NewSQLServerService(config.ConnStr)
		default:
			return nil, fmt.Errorf("unsupported database type: %s", config.DatabaseType)
		}
	default:
		return nil, fmt.Errorf("unsupported service type: %s", config.Type)
	}
}

// CSVService is a FHIRService implementation for translating CSV data to FHIR format
type SIMCSVService struct {
	FilePath string // Path to the CSV file
}

func NewSIMCSVService(filePath string) *SIMCSVService {
	return &SIMCSVService{
		FilePath: filePath,
	}
}

type Service interface {
	GetPatient(id string) (*fhir.Patient, error)
	GetAllPatients() ([]*fhir.Patient, error)
}

func (s *SIMCSVService) readCSVFile() (*os.File, *csv.Reader, error) {
	file, err := os.Open(s.FilePath)
	if err != nil {
		return nil, nil, err
	}

	reader := csv.NewReader(file)
	reader.Comma = ';'

	// Read and discard the header row
	if _, err := reader.Read(); err != nil {
		return nil, nil, err
	}

	return file, reader, nil
}

func (s *SIMCSVService) GetPatient(id string) (*fhir.Patient, error) {
	file, reader, err := s.readCSVFile()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		simPatient, err := mapRecordToSIMPatient(record)
		if err != nil {
			return nil, err
		}

		fhirPatient, err := TranslateSIMPatientToFHIR(simPatient)
		if err != nil {
			return nil, err
		}

		if *fhirPatient.Id == id {
			return fhirPatient, nil
		}
	}

	return nil, nil
}

func (s *SIMCSVService) GetAllPatients() ([]*fhir.Patient, error) {
	file, reader, err := s.readCSVFile()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var patients []*fhir.Patient
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		simPatient, err := mapRecordToSIMPatient(record)
		if err != nil {
			return nil, err
		}

		fhirPatient, err := TranslateSIMPatientToFHIR(simPatient)
		if err != nil {
			return nil, err
		}

		patients = append(patients, fhirPatient)
	}

	if len(patients) == 0 {
		return nil, fmt.Errorf("no patients found")
	}

	return patients, nil
}

type FHIRNDJSONService struct {
	FilePath string // Path to the NDJSON file
}

func NewFHIRNDJSONService(filePath string) *FHIRNDJSONService {
	return &FHIRNDJSONService{
		FilePath: filePath,
	}
}

func (s *FHIRNDJSONService) GetPatient(id string) (*fhir.Patient, error) {
	file, err := os.Open(s.FilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		var patient fhir.Patient
		if err := json.Unmarshal([]byte(line), &patient); err != nil {
			return nil, err
		}

		if *patient.Id == id {
			return &patient, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return nil, nil
}

func (s *FHIRNDJSONService) GetAllPatients() ([]*fhir.Patient, error) {
	file, err := os.Open(s.FilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var patients []*fhir.Patient

	for scanner.Scan() {
		line := scanner.Text()

		var patient fhir.Patient
		if err := json.Unmarshal([]byte(line), &patient); err != nil {
			return nil, err
		}

		patients = append(patients, &patient)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return patients, nil
}

type SQLService struct {
	db *gorm.DB
}

func NewSQLService(connStr string, databaseType string) (*SQLService, error) {
	db, err := gorm.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	return &SQLService{db: db}, nil
}

func (s *SQLService) GetPatient(id string) (*fhir.Patient, error) {
	var patient sim.Patient
	err := s.db.Raw("SELECT * FROM patient_hix_patient WHERE identificatienummer = ?", id).Scan(&patient).Error
	if err != nil {
		return nil, err
	}

	fhirPatient, err := TranslateSIMPatientToFHIR(&patient)
	if err != nil {
		return nil, err
	}

	return fhirPatient, nil
}

func (s *SQLService) GetAllPatients() ([]*fhir.Patient, error) {

	queryPath := util.GetAbsolutePath("queries/hix/patient.sql")

	query, err := os.ReadFile(queryPath)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Raw(string(query)).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fhirPatients []*fhir.Patient

	for rows.Next() {
		var patientJSON sql.NullString
		err := rows.Scan(&patientJSON)
		if err != nil {
			return nil, err
		}

		var patient fhir.Patient
		err = json.Unmarshal([]byte(patientJSON.String), &patient)
		if err != nil {
			return nil, err
		}

		fhirPatients = append(fhirPatients, &patient)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return fhirPatients, nil
}
