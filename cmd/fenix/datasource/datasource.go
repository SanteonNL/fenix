package datasource

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
)

// datasource.ResourceResult maps FHIR paths to their row data
type ResourceResult map[string][]RowData

// RowData represents a single row of data
type RowData struct {
	ID       string
	ParentID string
	Data     map[string]interface{}
}

// DataSourceService handles database operations and query management
type DataSourceService struct {
	db      *sqlx.DB
	queries map[string]string // resourceType -> query
	log     zerolog.Logger
}

// NewDataSourceService creates a new DataSourceService
func NewDataSourceService(db *sqlx.DB, log zerolog.Logger) *DataSourceService {
	return &DataSourceService{
		db:      db,
		queries: make(map[string]string),
		log:     log,
	}
}

// LoadQueryFile loads a single query file
func (svc *DataSourceService) LoadQueryFile(filePath string) error {
	// Get resource type from filename (e.g., "patient_query.sql" -> "Patient")
	fileName := filepath.Base(filePath)
	parts := strings.Split(fileName, "_")
	if len(parts) < 2 {
		return fmt.Errorf("invalid query file name format: %s", fileName)
	}

	resourceType := strings.TrimSuffix(
		strings.Title(parts[0]),
		filepath.Ext(fileName),
	)

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open query file %s: %w", filePath, err)
	}
	defer file.Close()

	query, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read query file %s: %w", filePath, err)
	}

	svc.queries[resourceType] = string(query)
	svc.log.Debug().
		Str("resourceType", resourceType).
		Str("file", filePath).
		Msg("Loaded query file")

	return nil
}

// LoadQueryDirectory loads all SQL files from a directory
func (svc *DataSourceService) LoadQueryDirectory(dirPath string) error {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read query directory %s: %w", dirPath, err)
	}

	var loadErrors []error
	loaded := 0

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".sql") {
			continue
		}

		filePath := filepath.Join(dirPath, file.Name())
		if err := svc.LoadQueryFile(filePath); err != nil {
			loadErrors = append(loadErrors, err)
			svc.log.Error().Err(err).
				Str("file", file.Name()).
				Msg("Failed to load query file")
			continue
		}
		loaded++
	}

	svc.log.Info().
		Int("total_files", len(files)).
		Int("loaded", loaded).
		Int("errors", len(loadErrors)).
		Str("directory", dirPath).
		Msg("Completed loading query files")

	if len(loadErrors) > 0 {
		return fmt.Errorf("encountered %d errors while loading query files", len(loadErrors))
	}

	return nil
}

// GetQuery retrieves a query for a resource type
func (svc *DataSourceService) GetQuery(resourceType string) (string, error) {
	query, exists := svc.queries[resourceType]
	if !exists {
		return "", fmt.Errorf("no query found for resource type: %s", resourceType)
	}
	return query, nil
}

// ReadResources reads resources from the database using the stored query
func (svc *DataSourceService) ReadResources(resourceType, patientID string) ([]ResourceResult, error) {
	query, err := svc.GetQuery(resourceType)
	if err != nil {
		return nil, err
	}

	// Replace parameter in query
	query = strings.ReplaceAll(query,
		fmt.Sprintf(":%s.id", resourceType),
		fmt.Sprintf("'%s'", patientID))

	rows, err := svc.db.Queryx(query)
	if err != nil {
		return nil, fmt.Errorf("error executing query: %w", err)
	}
	defer rows.Close()

	// Map to hold resources by their resource_id
	resources := make(map[string]ResourceResult)

	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}

		// Remove NULL values
		for key, value := range row {
			if value == nil {
				delete(row, key)
			}
		}

		svc.processRow(row, resources)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	// Convert map to slice
	results := make([]ResourceResult, 0, len(resources))
	for _, result := range resources {
		results = append(results, result)
	}

	return results, nil
}

func (ds *DataSourceService) processRow(row map[string]interface{}, resources map[string]ResourceResult) {
	// Extract metadata fields
	id, _ := row["id"].(string)
	parentID, _ := row["parent_id"].(string)
	fhirPath, _ := row["fhir_path"].(string)
	resourceID, _ := row["resource_id"].(string)

	ds.log.Debug().
		Str("id", id).
		Str("fhirPath", fhirPath).
		Str("resourceID", resourceID).
		Msg("Processing row")

	// Initialize resource result if needed
	if resources[resourceID] == nil {
		resources[resourceID] = make(ResourceResult)
	}

	// Process top-level fields
	topLevelData := make(map[string]interface{})

	// Add ID to the Data map
	topLevelData["id"] = id

	for key, value := range row {
		// Skip if value is nil
		if value == nil {
			continue
		}

		// Skip metadata fields except for id which we want to include
		if key == "parent_id" || key == "fhir_path" || key == "resource_id" {
			continue
		}

		// Handle based on whether field contains dots (nested) or not
		if !strings.Contains(key, ".") {
			// Top level field
			topLevelData[key] = value
		} else {
			// Nested field, process separately
			parts := strings.Split(key, ".")
			ds.processNestedField(parts, value, id, parentID, fhirPath, resourceID, resources)
		}
	}

	// Handle top-level fields if any exist
	if len(topLevelData) > 0 {
		// Initialize slice for this path if needed
		if resources[resourceID][fhirPath] == nil {
			resources[resourceID][fhirPath] = []RowData{}
		}

		// Try to find existing entry
		existingIndex := -1
		for idx, existingRow := range resources[resourceID][fhirPath] {
			if existingRow.ID == id {
				existingIndex = idx
				break
			}
		}

		if existingIndex != -1 {
			// Update existing entry
			for k, v := range topLevelData {
				resources[resourceID][fhirPath][existingIndex].Data[k] = v
			}
			ds.log.Debug().
				Str("id", id).
				Str("path", fhirPath).
				Msg("Updated existing entry")
		} else {
			// Add new entry
			resources[resourceID][fhirPath] = append(resources[resourceID][fhirPath], RowData{
				ID:       id,
				ParentID: parentID,
				Data:     topLevelData,
			})
			ds.log.Debug().
				Str("id", id).
				Str("path", fhirPath).
				Msg("Added new entry")
		}
	}
}

// Corrected processNestedField function using resource_id
func (ds *DataSourceService) processNestedField(
	parts []string,
	value interface{},
	id string,
	parentID string,
	basePath string,
	resourceID string,
	resources map[string]ResourceResult,
) {
	currentPath := basePath
	currentID := id
	currentParentID := parentID

	// Get the root object ID from the resource_id
	rootID := resourceID
	// If there's a root object already in the result, use its ID
	if rootRows, exists := resources[resourceID][basePath]; exists && len(rootRows) > 0 {
		rootID = rootRows[0].ID
	}

	ds.log.Debug().
		Str("resourceID", resourceID).
		Str("rootID", rootID).
		Str("basePath", basePath).
		Msg("Starting nested field processing")

	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]

		// Extract any array index and clean the part name
		arrayIndex := ds.extractIndex(part)
		cleanPart := ds.removeIndex(part)

		// Build path without array index
		currentPath += "." + cleanPart

		// For first level nested fields, set parent ID to resource's root object ID
		if i == 0 {
			// FIX: Set parentID to root object ID for first level
			currentParentID = rootID

			if arrayIndex >= 0 {
				// First level with array index (e.g., "category[0]")
				currentID = fmt.Sprintf("%s_%d", rootID, arrayIndex+1)
			} else {
				// First level without array index (e.g., "code")
				currentID = fmt.Sprintf("%s_%d", rootID, 1)
			}

			ds.log.Debug().
				Str("path", currentPath).
				Str("rootID", rootID).
				Str("currentID", currentID).
				Str("currentParentID", currentParentID).
				Msg("Set first-level field parentID to resource root ID")
		} else {
			// Nested levels (below first level)
			if arrayIndex >= 0 {
				// Nested array elements (e.g., "code.coding[0]")
				currentID = fmt.Sprintf("%s_%d", currentParentID, arrayIndex+1)
			}
			// For nested elements under an array item, prefix with parent ID
			if i > 0 && strings.Contains(parts[i-1], "[") {
				currentID = fmt.Sprintf("%s_%d", currentParentID, arrayIndex+1)
			}
		}

		ds.log.Debug().
			Str("part", part).
			Str("path", currentPath).
			Str("id", currentID).
			Str("parentID", currentParentID).
			Msg("Processing nested field part")

		// Check if we're at the leaf level
		isLeaf := i >= len(parts)-2

		// Initialize path if needed
		if resources[resourceID][currentPath] == nil {
			resources[resourceID][currentPath] = []RowData{}
		}

		// Find existing entry or create new one
		existingIndex := -1
		for idx, row := range resources[resourceID][currentPath] {
			if row.ID == currentID {
				existingIndex = idx
				break
			}
		}

		if isLeaf {
			// Handle leaf node
			leafField := parts[len(parts)-1]

			if existingIndex != -1 {
				// Update existing entry
				resources[resourceID][currentPath][existingIndex].Data[leafField] = value
				// Ensure ID is in Data
				resources[resourceID][currentPath][existingIndex].Data["id"] = currentID
				// FIX: Ensure parentID is set correctly
				resources[resourceID][currentPath][existingIndex].ParentID = currentParentID
			} else {
				// Create new entry with ID in Data
				newRow := RowData{
					ID:       currentID,
					ParentID: currentParentID, // FIX: Ensure parentID is set correctly
					Data: map[string]interface{}{
						leafField: value,
						"id":      currentID,
					},
				}
				resources[resourceID][currentPath] = append(resources[resourceID][currentPath], newRow)
			}
		} else if existingIndex == -1 {
			// Create new intermediate node with ID in Data
			newRow := RowData{
				ID:       currentID,
				ParentID: currentParentID, // FIX: Ensure parentID is set correctly
				Data: map[string]interface{}{
					"id": currentID,
				},
			}
			resources[resourceID][currentPath] = append(resources[resourceID][currentPath], newRow)
		} else {
			// Ensure ID is in Data for existing nodes
			resources[resourceID][currentPath][existingIndex].Data["id"] = currentID
			// FIX: Ensure parentID is set correctly
			resources[resourceID][currentPath][existingIndex].ParentID = currentParentID
		}

		// Update parent ID for next iteration
		currentParentID = currentID
	}
}

func (ds *DataSourceService) extractIndex(part string) int {
	start := strings.Index(part, "[")
	end := strings.Index(part, "]")
	if start != -1 && end != -1 && start < end {
		index, err := strconv.Atoi(part[start+1 : end])
		if err == nil {
			return index
		}
	}
	return 0
}

func (ds *DataSourceService) removeIndex(part string) string {
	index := strings.Index(part, "[")
	if index != -1 {
		return part[:index]
	}
	return part
}

// findSQLFilesInDir recursively searches for SQL files in a directory
func (ds *DataSourceService) FindSQLFilesInDir(dir string, resourceType string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			subFiles, err := ds.FindSQLFilesInDir(path, resourceType)
			if err != nil {
				continue
			}
			files = append(files, subFiles...)
		} else if strings.HasPrefix(strings.ToLower(entry.Name()), strings.ToLower(resourceType)) &&
			strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			files = append(files, path)
		}
	}

	return files, nil
}

// Example usage:
func Example() {
	// Initialize database connection
	db, err := sqlx.Connect("postgres", "postgres://postgres:mysecretpassword@localhost:5432/tsl_employee?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}

	// Create logger
	logger := zerolog.New(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = os.Stdout
		w.TimeFormat = time.Kitchen
	})).With().Timestamp().Caller().Logger()

	// Initialize service
	service := NewDataSourceService(db, logger)

	// Load queries
	err = service.LoadQueryFile("queries/hix/flat/patient_1.sql")
	if err != nil {
		log.Fatal(err)
	}

	// Read resources
	results, err := service.ReadResources("Patient", "12345")
	if err != nil {
		log.Printf("Error: %v", err)
	}

	// Print results
	for _, result := range results {
		for path, rowData := range result {
			fmt.Printf("Resource Path: %s, Data: %v\n", path, rowData)
		}
	}
}
