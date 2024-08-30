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
	log := zerolog.New(os.Stdout).With().Timestamp().Logger()
	log.Debug().Str("filePath", filePath).Msg("Attempting to load CSV mapper config")

	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Error().Err(err).Str("filePath", filePath).Msg("Config file does not exist")
		return nil, fmt.Errorf("config file does not exist: %w", err)
	}

	// Check if we have read permissions
	file, err := os.Open(filePath)
	if err != nil {
		log.Error().Err(err).Str("filePath", filePath).Msg("Unable to open config file")
		return nil, fmt.Errorf("unable to open config file: %w", err)
	}
	file.Close()

	// Attempt to read the file
	jsonFile, err := os.ReadFile(filePath)
	if err != nil {
		log.Error().Err(err).Str("filePath", filePath).Msg("Failed to read config file")
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Check if the file is empty
	if len(jsonFile) == 0 {
		log.Error().Str("filePath", filePath).Msg("Config file is empty")
		return nil, fmt.Errorf("config file is empty: %s", filePath)
	}

	var mapper CSVMapper
	err = json.Unmarshal(jsonFile, &mapper)
	if err != nil {
		log.Error().Err(err).Str("filePath", filePath).Msg("Failed to unmarshal config file")
		return nil, fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	log.Debug().Str("filePath", filePath).Msg("Successfully loaded CSV mapper config")
	return &mapper, nil
}

func (c *CSVDataSource) Read(resourceName string) (map[string]map[string][]map[string]interface{}, error) {
	c.log.Info().Str("resourceName", resourceName).Msg("Starting to read CSV data for resource")
	result := make(map[string]map[string][]map[string]interface{})
	idMap := make(map[string]map[string]string) // map[FHIRPath]map[ID]PatientID

	for _, mapping := range c.mapper.Mappings {
		fhirPathParts := strings.Split(mapping.FHIRPath, ".")
		if len(fhirPathParts) == 0 || fhirPathParts[0] != resourceName {
			c.log.Debug().Str("fhirPath", mapping.FHIRPath).Msg("Skipping mapping, not for requested resource")
			continue
		}

		c.log.Debug().Str("fhirPath", mapping.FHIRPath).Msg("Processing mapping")
		idMap[mapping.FHIRPath] = make(map[string]string)
		for _, fileMapping := range mapping.Files {
			c.log.Debug().Str("fileName", fileMapping.FileName).Msg("Reading file")
			fileData, err := c.readFile(fileMapping.FileName)
			if err != nil {
				c.log.Error().Err(err).Str("fileName", fileMapping.FileName).Msg("Failed to read file")
				return nil, err
			}
			c.log.Debug().Str("fileName", fileMapping.FileName).Int("rowCount", len(fileData)).Msg("File read successfully")

			for rowIndex, row := range fileData {
				for _, fieldMapping := range fileMapping.FieldMappings {
					mappedRow := make(map[string]interface{})

					for csvField, fhirField := range fieldMapping.CSVFields {
						if value, ok := row[csvField]; ok {
							mappedRow[fhirField] = value
						}
					}

					id, ok := row[fieldMapping.IDField].(string)
					if !ok {
						c.log.Warn().Str("fhirPath", mapping.FHIRPath).Int("rowIndex", rowIndex).Msg("ID not found or not a string")
						continue
					}
					mappedRow["id"] = id

					if fieldMapping.ParentIDField != "" {
						if parentID, ok := row[fieldMapping.ParentIDField].(string); ok {
							mappedRow["parent_id"] = parentID
						}
					}

					mappedRow["field_name"] = mapping.FHIRPath

					if len(mappedRow) <= 3 {
						c.log.Debug().Str("fhirPath", mapping.FHIRPath).Int("rowIndex", rowIndex).Msg("Skipping row, no fields mapped")
						continue
					}

					patientID := c.resolvePatientID(mapping.FHIRPath, row, fieldMapping, idMap)
					if patientID == "" {
						c.log.Warn().Str("fhirPath", mapping.FHIRPath).Str("id", id).Int("rowIndex", rowIndex).Msg("Patient ID not found")
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

	c.log.Info().Str("resourceName", resourceName).Int("patientCount", len(result)).Msg("Completed reading CSV data")
	return result, nil
}

func (c *CSVDataSource) readFile(fileName string) ([]map[string]interface{}, error) {
	c.log.Debug().Str("fileName", fileName).Msg("Starting to read CSV file")
	file, err := os.Open(fileName)
	if err != nil {
		c.log.Error().Err(err).Str("fileName", fileName).Msg("Failed to open file")
		return nil, fmt.Errorf("failed to open file %s: %w", fileName, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ';'
	headers, err := reader.Read()
	if err != nil {
		c.log.Error().Err(err).Str("fileName", fileName).Msg("Failed to read headers")
		return nil, fmt.Errorf("failed to read headers from file %s: %w", fileName, err)
	}
	c.log.Debug().Str("fileName", fileName).Strs("headers", headers).Msg("CSV headers read")

	var result []map[string]interface{}
	lineCount := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			c.log.Error().Err(err).Str("fileName", fileName).Int("line", lineCount).Msg("Failed to read record")
			return nil, fmt.Errorf("failed to read record from file %s at line %d: %w", fileName, lineCount, err)
		}

		row := make(map[string]interface{})
		for i, value := range record {
			row[headers[i]] = value
		}

		result = append(result, row)
		lineCount++

		if lineCount%1000 == 0 {
			c.log.Debug().Str("fileName", fileName).Int("linesProcessed", lineCount).Msg("Processing CSV lines")
		}
	}

	c.log.Info().Str("fileName", fileName).Int("totalLines", lineCount).Msg("Finished reading CSV file")
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
