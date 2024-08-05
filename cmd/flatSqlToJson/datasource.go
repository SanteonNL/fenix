package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

type DataSource interface {
	Read() (map[string][]map[string]interface{}, error)
}

type SQLDataSource struct {
	db    *sqlx.DB
	query string
}

func NewSQLDataSource(db *sqlx.DB, query string) *SQLDataSource {
	return &SQLDataSource{
		db:    db,
		query: query,
	}
}

type CSVDataSource struct {
	filePath string
	mapper   *CSVMapper
}

type CSVMapper struct {
	Mappings []CSVMapping
}

type CSVMapping struct {
	FieldName string
	Files     []CSVFileMapping
}

type CSVFileMapping struct {
	FileName      string
	FieldMappings []CSVFieldMapping
}

type CSVFieldMapping struct {
	FHIRField     map[string]string
	ParentIDField string
	IDField       string
}

type CSVMapperConfig struct {
	Mappings []struct {
		FieldName string `json:"fieldName"`
		Files     []struct {
			FileName      string `json:"fileName"`
			FieldMappings []struct {
				CSVFields     map[string]string `json:"csvFields"`
				IDField       string            `json:"idField"`
				ParentIDField string            `json:"parentIdField"`
			} `json:"fieldMappings"`
		} `json:"files"`
	} `json:"mappings"`
}

func LoadCSVMapperFromConfig(filePath string) (*CSVMapper, error) {
	jsonFile, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config CSVMapperConfig
	err = json.Unmarshal(jsonFile, &config)
	if err != nil {
		return nil, err
	}

	mapper := &CSVMapper{
		Mappings: make([]CSVMapping, len(config.Mappings)),
	}

	for i, configMapping := range config.Mappings {
		mapping := CSVMapping{
			FieldName: configMapping.FieldName,
			Files:     make([]CSVFileMapping, len(configMapping.Files)),
		}

		for j, configFile := range configMapping.Files {
			fileMapping := CSVFileMapping{
				FileName:      configFile.FileName,
				FieldMappings: make([]CSVFieldMapping, len(configFile.FieldMappings)),
			}

			for k, configFieldMapping := range configFile.FieldMappings {
				fieldMapping := CSVFieldMapping{
					FHIRField:     configFieldMapping.CSVFields,
					IDField:       configFieldMapping.IDField,
					ParentIDField: configFieldMapping.ParentIDField,
				}
				fileMapping.FieldMappings[k] = fieldMapping
			}

			mapping.Files[j] = fileMapping
		}

		mapper.Mappings[i] = mapping
	}

	return mapper, nil
}

func (s *SQLDataSource) Read() (map[string][]map[string]interface{}, error) {
	result := make(map[string][]map[string]interface{})

	rows, err := s.db.Queryx(s.query)
	if err != nil {
		return nil, fmt.Errorf("error executing query: %w", err)
	}
	defer rows.Close()

	resultSetCount := 0
	for {
		resultSetCount++
		log.Debug().Int("resultSet", resultSetCount).Msg("Processing result set")

		rowCount := 0
		for rows.Next() {
			rowCount++
			row := make(map[string]interface{})
			err = rows.MapScan(row)
			if err != nil {
				return nil, fmt.Errorf("error scanning row: %w", err)
			}

			log.Debug().Int("resultSet", resultSetCount).Int("row", rowCount).Interface("row", row).Msg("Row from SQL query")

			// Remove NULL values
			for key, value := range row {
				if value == nil {
					delete(row, key)
				}
			}

			fieldName, ok := row["field_name"].(string)
			if !ok {
				return nil, fmt.Errorf("field_name not found or not a string in result set %d, row %d", resultSetCount, rowCount)
			}

			delete(row, "field_name")
			result[fieldName] = append(result[fieldName], row)
		}

		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating over rows in result set %d: %w", resultSetCount, err)
		}

		log.Debug().Int("resultSet", resultSetCount).Int("rowCount", rowCount).Msg("Finished processing result set")

		// Move to the next result set
		if !rows.NextResultSet() {
			break // No more result sets
		}
	}

	log.Debug().Int("resultSets", resultSetCount).Interface("result", result).Msg("Data from SQL query")
	return result, nil
}

func NewCSVDataSource(filePath string, mapper *CSVMapper) *CSVDataSource {
	return &CSVDataSource{
		filePath: filePath,
		mapper:   mapper,
	}
}

func (c *CSVDataSource) Read() (map[string][]map[string]interface{}, error) {
	result := make(map[string][]map[string]interface{})

	for _, mapping := range c.mapper.Mappings {
		for _, fileMapping := range mapping.Files {
			fileData, err := c.readFile(fileMapping)
			if err != nil {
				return nil, err
			}
			result[mapping.FieldName] = append(result[mapping.FieldName], fileData...)
		}
	}

	return result, nil
}

func (c *CSVDataSource) readFile(fileMapping CSVFileMapping) ([]map[string]interface{}, error) {
	file, err := os.Open(fileMapping.FileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ';'
	headers, err := reader.Read()
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, header := range headers {
			for _, fieldMapping := range fileMapping.FieldMappings {
				if fhirField, ok := fieldMapping.FHIRField[header]; ok {
					if record[i] != "" {
						row[fhirField] = record[i]
					}
				}
				if header == fieldMapping.ParentIDField {
					row["parent_id"] = record[i]
				}
				if header == fieldMapping.IDField {
					row["id"] = record[i]
				}
			}
		}

		result = append(result, row)
	}

	return result, nil
}
