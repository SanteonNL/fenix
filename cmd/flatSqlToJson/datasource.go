package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

type DataSource interface {
	Read(string) (map[string]map[string][]map[string]interface{}, error) // map[patientID\map[fhirPath][]map[fhirField]fhirValue
}

type SQLDataSource struct {
	db    *sqlx.DB
	query string
	log   zerolog.Logger
}

func NewSQLDataSource(db *sqlx.DB, query string, log zerolog.Logger) *SQLDataSource {
	return &SQLDataSource{
		db:    db,
		query: query,
		log:   log,
	}
}

type CSVDataSource struct {
	mapper *CSVMapper
	log    zerolog.Logger
}

type CSVMapper struct {
	Mappings []CSVMapping `json:"mappings"`
}

type CSVMapping struct {
	FHIRPath string           `json:"fhirPath"`
	Files    []CSVFileMapping `json:"files"`
}

type CSVFileMapping struct {
	FileName      string            `json:"fileName"`
	FieldMappings []CSVFieldMapping `json:"fieldMappings"`
}

type CSVFieldMapping struct {
	CSVFields      map[string]string `json:"csvFields"`
	IDField        string            `json:"idField"`
	ParentIDField  string            `json:"parentIdField"`
	PatientIDField string            `json:"patientIdField"`
}

func NewCSVDataSource(configFilePath string, log zerolog.Logger) (*CSVDataSource, error) {
	mapper, err := LoadCSVMapperFromConfig(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load CSV mapper config: %w", err)
	}

	return &CSVDataSource{
		mapper: mapper,
		log:    log,
	}, nil
}

func LoadCSVMapperFromConfig(filePath string) (*CSVMapper, error) {
	jsonFile, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var mapper CSVMapper
	err = json.Unmarshal(jsonFile, &mapper)
	if err != nil {
		return nil, err
	}

	return &mapper, nil
}

func (c *CSVDataSource) Read(resourceName string) (map[string]map[string][]map[string]interface{}, error) {
	result := make(map[string]map[string][]map[string]interface{})
	idMap := make(map[string]map[string]string) // map[FHIRPath]map[ID]PatientID

	for _, mapping := range c.mapper.Mappings {
		// Check if the mapping is for the requested resource
		fhirPathParts := strings.Split(mapping.FHIRPath, ".")
		if len(fhirPathParts) == 0 || fhirPathParts[0] != resourceName {
			continue
		}

		idMap[mapping.FHIRPath] = make(map[string]string)
		for _, fileMapping := range mapping.Files {
			fileData, err := c.readFile(fileMapping.FileName)
			if err != nil {
				return nil, err
			}

			for _, row := range fileData {
				for _, fieldMapping := range fileMapping.FieldMappings {
					mappedRow := make(map[string]interface{})

					// Map CSV fields to FHIR fields
					for csvField, fhirField := range fieldMapping.CSVFields {
						if value, ok := row[csvField]; ok {
							mappedRow[fhirField] = value
						}
					}

					// Handle special fields
					id, ok := row[fieldMapping.IDField].(string)
					if !ok {
						c.log.Warn().Str("fhirPath", mapping.FHIRPath).Msg("ID not found or not a string")
						continue
					}
					mappedRow["id"] = id

					if fieldMapping.ParentIDField != "" {
						if parentID, ok := row[fieldMapping.ParentIDField].(string); ok {
							mappedRow["parent_id"] = parentID
						}
					}

					mappedRow["field_name"] = mapping.FHIRPath

					// If no fields were mapped, skip this row
					if len(mappedRow) <= 3 { // Only id, parent_id, and field_name
						continue
					}

					patientID := c.resolvePatientID(mapping.FHIRPath, row, fieldMapping, idMap)
					if patientID == "" {
						c.log.Warn().Str("fhirPath", mapping.FHIRPath).Str("id", id).Msg("Patient ID not found")
						continue
					}

					idMap[mapping.FHIRPath][id] = patientID

					if result[patientID] == nil {
						result[patientID] = make(map[string][]map[string]interface{})
					}
					result[patientID][mapping.FHIRPath] = append(result[patientID][mapping.FHIRPath], mappedRow)
				}
			}
		}
	}

	return result, nil
}
func (c *CSVDataSource) readFile(fileName string) ([]map[string]interface{}, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", fileName, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ';'
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read headers from file %s: %w", fileName, err)
	}

	var result []map[string]interface{}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read record from file %s: %w", fileName, err)
		}

		row := make(map[string]interface{})
		for i, value := range record {
			row[headers[i]] = value
		}

		result = append(result, row)
	}

	return result, nil
}

func (c *CSVDataSource) resolvePatientID(fhirPath string, row map[string]interface{}, fieldMapping CSVFieldMapping, idMap map[string]map[string]string) string {
	if patientID, ok := row[fieldMapping.PatientIDField].(string); ok && patientID != "" {
		return patientID
	}

	parentPath := fhirPath[:strings.LastIndex(fhirPath, ".")]
	if parentMap, ok := idMap[parentPath]; ok {
		if parentID, ok := row[fieldMapping.ParentIDField].(string); ok {
			if patientID, ok := parentMap[parentID]; ok {
				return patientID
			}
		}
	}

	return ""
}

func (s *SQLDataSource) Read(patientID string) (map[string]map[string][]map[string]interface{}, error) {
	result := make(map[string]map[string][]map[string]interface{})

	rows, err := s.db.Queryx(s.query)
	if err != nil {
		return nil, fmt.Errorf("error executing query: %w", err)
	}
	defer rows.Close()

	for {
		for rows.Next() {
			row := make(map[string]interface{})
			err = rows.MapScan(row)
			if err != nil {
				return nil, fmt.Errorf("error scanning row: %w", err)
			}

			// Remove NULL values
			for key, value := range row {
				if value == nil {
					delete(row, key)
				}
			}

			fieldName, ok := row["field_name"].(string)
			if !ok {
				return nil, fmt.Errorf("field_name not found or not a string in row")
			}
			delete(row, "field_name")

			uniqueID, ok := row["id"].(string)
			if !ok {
				return nil, fmt.Errorf("id not found or not a string in row")
			}

			if result[patientID] == nil {
				result[patientID] = make(map[string][]map[string]interface{})
			}

			// Check if this uniqueID already exists for this fieldName
			found := false
			for i, existingRow := range result[patientID][fieldName] {
				if existingID, ok := existingRow["id"].(string); ok && existingID == uniqueID {
					// Update existing row instead of appending a new one
					result[patientID][fieldName][i] = row
					found = true
					break
				}
			}

			// If not found, append as a new row
			if !found {
				result[patientID][fieldName] = append(result[patientID][fieldName], row)
			}
		}

		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating over rows: %w", err)
		}

		// Move to the next result set
		if !rows.NextResultSet() {
			break // No more result sets
		}
	}

	return result, nil
}
