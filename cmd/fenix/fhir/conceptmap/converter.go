// internal/fhir/conceptmap/converter.go
package conceptmap

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

// ConceptMapConverter handles conversion of mapping files to FHIR ConceptMaps
type ConceptMapConverter struct {
	log               zerolog.Logger
	conceptMapService *ConceptMapService
}

// NewConceptMapConverter creates a new converter instance
func NewConceptMapConverter(log zerolog.Logger, conceptMapService *ConceptMapService) *ConceptMapConverter {
	return &ConceptMapConverter{
		log:               log,
		conceptMapService: conceptMapService,
	}
}

// ConvertFolderToFHIR converts all CSV files in a folder to FHIR ConceptMaps
func (c *ConceptMapConverter) ConvertFolderToFHIR(inputFolder string, repository *ConceptMapRepository, usePrefix bool) error {
	files, err := os.ReadDir(inputFolder)
	if err != nil {
		return fmt.Errorf("failed to read input directory: %w", err)
	}

	var conversionErrors []string
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(strings.ToLower(file.Name()), ".csv") {
			continue
		}

		filePath := filepath.Join(inputFolder, file.Name())
		csvFile, err := os.Open(filePath)
		if err != nil {
			conversionErrors = append(conversionErrors, fmt.Sprintf("failed to open %s: %v", file.Name(), err))
			continue
		}

		err = c.ConvertCSVToFHIRAndSave(csvFile, file.Name(), repository, usePrefix)
		csvFile.Close()
		if err != nil {
			conversionErrors = append(conversionErrors, fmt.Sprintf("failed to convert %s: %v", file.Name(), err))
			continue
		}

		c.conceptMapService.log.Info().
			Str("file", file.Name()).
			Msg("Successfully converted CSV to ConceptMap")
	}

	if len(conversionErrors) > 0 {
		return fmt.Errorf("encountered errors during conversion:\n%s", strings.Join(conversionErrors, "\n"))
	}

	return nil
}

// ConvertCSVToFHIRAndSave converts a CSV file to a FHIR ConceptMap and saves it to the repository's converted folder
func (c *ConceptMapConverter) ConvertCSVToFHIRAndSave(reader io.Reader, csvName string, repository *ConceptMapRepository, usePrefix bool) error {
	csvReader := csv.NewReader(reader)
	csvReader.Comma = ';'
	csvReader.TrimLeadingSpace = true

	// Read and validate headers
	headers, err := csvReader.Read()
	if err != nil {
		return fmt.Errorf("failed to read headers: %w", err)
	}

	indices := getColumnIndices(headers)
	if !indices.areValid() {
		return fmt.Errorf("required columns not found in CSV")
	}

	// Remove .csv extension if present
	baseName := strings.TrimSuffix(csvName, filepath.Ext(csvName))

	// Create initial ConceptMap
	conceptMap := c.conceptMapService.CreateConceptMap(
		fmt.Sprintf("%s_%s", baseName, time.Now().Format("20060102")),
		baseName,
		"", // Will be populated from first row
		"", // Will be populated from first row
	)

	// Track processed systems for efficient grouping
	groupMap := make(map[string]*fhir.ConceptMapGroup)

	// Process each row
	var firstRowProcessed bool
	for {
		row, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read row: %w", err)
		}

		mapping, err := c.extractMapping(row, indices)
		if err != nil {
			return fmt.Errorf("failed to extract mapping from row: %w", err)
		}

		// Update ValueSet URI from first valid row
		if !firstRowProcessed && mapping.ValueSetURI != "" {
			uri := mapping.ValueSetURI
			// if usePrefix {
			// 	// Add prefix to the last segment of the URI
			// 	segments := strings.Split(uri, "/")
			// 	if len(segments) > 0 {
			// 		lastIndex := len(segments) - 1
			// 		segments[lastIndex] = "conceptmap_converted_" + segments[lastIndex]
			// 		uri = strings.Join(segments, "/")
			// 	}
			// }
			conceptMap.TargetUri = &uri
			firstRowProcessed = true
		}

		// Add mapping to ConceptMap
		if err := c.addMappingToConceptMap(conceptMap, mapping, groupMap); err != nil {
			return fmt.Errorf("failed to add mapping: %w", err)
		}
	}

	// Convert groupMap to final groups slice
	conceptMap.Group = c.finalizeGroups(groupMap)

	// Create the fhir/converted directory within the repository's local path
	outputPath := filepath.Join(repository.localPath, "fhir", "converted")
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Save with original CSV name (minus extension) plus .json
	outputFile := filepath.Join(outputPath, baseName+".json")
	if err := c.conceptMapService.SaveConceptMap(outputFile, conceptMap); err != nil {
		return fmt.Errorf("failed to save ConceptMap: %w", err)
	}

	// Add to repository cache
	if conceptMap.Id != nil {
		repository.cache.Store(*conceptMap.Id, conceptMap)
	}
	if conceptMap.TargetUri != nil {
		repository.cache.Store(*conceptMap.TargetUri, conceptMap)
	}

	return nil
}

// ConvertCSVToFHIR maintains backwards compatibility while using the repository structure
func (c *ConceptMapConverter) ConvertCSVToFHIR(reader io.Reader, name string) (*fhir.ConceptMap, error) {

	// Create a temporary repository
	tempDir, err := os.MkdirTemp("", "conceptmap_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	tempRepo := NewConceptMapRepository(tempDir, c.conceptMapService.log)

	if err := c.ConvertCSVToFHIRAndSave(reader, name, tempRepo, false); err != nil {
		return nil, err
	}

	// Read the saved file
	convertedPath := filepath.Join(tempDir, "fhir", "converted")
	files, err := os.ReadDir(convertedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read converted directory: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no ConceptMap file was created")
	}

	data, err := os.ReadFile(filepath.Join(convertedPath, files[0].Name()))
	if err != nil {
		c.log.Fatal().Err(err).Msg("Failed to read ConceptMap file")
		return nil, fmt.Errorf("failed to read ConceptMap file: %w", err)
	}

	var conceptMap fhir.ConceptMap
	if err := json.Unmarshal(data, &conceptMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ConceptMap: %w", err)
	}

	return &conceptMap, nil
}

// extractMapping creates a CSVMapping from a CSV row
func (c *ConceptMapConverter) extractMapping(row []string, indices columnIndices) (*CSVMapping, error) {
	if len(row) <= indices.maxIndex() {
		return nil, fmt.Errorf("row has insufficient columns")
	}

	mapping := &CSVMapping{
		SourceSystem:  strings.TrimSpace(row[indices.sourceSystem]),
		SourceCode:    strings.TrimSpace(row[indices.sourceCode]),
		SourceDisplay: strings.TrimSpace(row[indices.sourceDisplay]),
		TargetSystem:  strings.TrimSpace(row[indices.targetSystem]),
		TargetCode:    strings.TrimSpace(row[indices.targetCode]),
		TargetDisplay: strings.TrimSpace(row[indices.targetDisplay]),
	}

	// Validate required fields
	if mapping.SourceCode == "" || mapping.TargetCode == "" {
		return nil, fmt.Errorf("source and target codes are required")
	}

	// Add ValueSet URI if available
	if indices.valueSetURI != -1 && indices.valueSetURI < len(row) {
		mapping.ValueSetURI = strings.TrimSpace(row[indices.valueSetURI])
	}

	return mapping, nil
}

// addMappingToConceptMap adds a mapping to the ConceptMap, handling group organization
func (c *ConceptMapConverter) addMappingToConceptMap(
	conceptMap *fhir.ConceptMap,
	mapping *CSVMapping,
	groupMap map[string]*fhir.ConceptMapGroup,
) error {
	// Create group key from source and target systems
	groupKey := fmt.Sprintf("%s|%s", mapping.SourceSystem, mapping.TargetSystem)

	// Get or create group
	group, exists := groupMap[groupKey]
	if !exists {
		group = &fhir.ConceptMapGroup{
			Source:  &mapping.SourceSystem,
			Target:  &mapping.TargetSystem,
			Element: make([]fhir.ConceptMapGroupElement, 0),
		}
		groupMap[groupKey] = group
	}

	// Create new element
	element := fhir.ConceptMapGroupElement{
		Code:    &mapping.SourceCode,
		Display: &mapping.SourceDisplay,
		Target: []fhir.ConceptMapGroupElementTarget{
			{
				Code:        &mapping.TargetCode,
				Display:     &mapping.TargetDisplay,
				Equivalence: 2, // equivalent by default
			},
		},
	}

	// Add element to group
	group.Element = append(group.Element, element)

	return nil
}

// finalizeGroups converts the group map to a sorted slice of groups
func (c *ConceptMapConverter) finalizeGroups(groupMap map[string]*fhir.ConceptMapGroup) []fhir.ConceptMapGroup {
	groups := make([]fhir.ConceptMapGroup, 0, len(groupMap))
	for _, group := range groupMap {
		groups = append(groups, *group)
	}
	return groups
}
