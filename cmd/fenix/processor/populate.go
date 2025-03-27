package processor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/SanteonNL/fenix/cmd/fenix/datasource"
	"github.com/SanteonNL/fenix/cmd/fenix/types"
	"github.com/SanteonNL/fenix/models/fhir"
)

// populateResourceStruct begint het vulproces voor het hoofdresource
func (p *ProcessorService) populateResourceStruct(value reflect.Value, filter []*types.Filter) (bool, error) {
	p.processedPaths = make(map[string]bool)
	return p.determinePopulateType(p.resourceType, value, "", filter)
}

// determinePopulateType bepaalt hoe een veld gevuld moet worden op basis van het type
func (p *ProcessorService) determinePopulateType(structPath string, value reflect.Value, parentID string, filter []*types.Filter) (bool, error) {
	p.log.Debug().
		Str("structPath", structPath).
		Str("type", value.Type().String()).
		Str("kind", value.Kind().String()).
		Str("parentID", parentID).
		Msg("Determining populate type")

	rows, exists := p.result[structPath]
	if !exists {
		p.log.Debug().
			Str("structPath", structPath).
			Msg("No rows found for path")
		return true, nil
	}

	switch value.Kind() {
	case reflect.Slice:
		return p.populateSlice(structPath, value, parentID, rows, filter)
	case reflect.Struct:
		return p.populateStruct(structPath, value, parentID, rows, filter)
	case reflect.Ptr:
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		return p.determinePopulateType(structPath, value.Elem(), parentID, filter)
	default:
		return p.setBasicType(structPath, value, parentID, rows, filter)
	}
}

// populateSlice vult een array-veld met elementen
func (p *ProcessorService) populateSlice(structPath string, value reflect.Value, parentID string, rows []datasource.RowData, filter []*types.Filter) (bool, error) {
	p.log.Debug().
		Str("structPath", structPath).
		Str("parentID", parentID).
		Int("total_rows", len(rows)).
		Msg("Populating slice")

	// Markeer dit pad als verwerkt
	p.processedPaths[structPath] = true

	// Maak slice om alle elementen te bevatten
	allElements := reflect.MakeSlice(value.Type(), 0, len(rows))
	anyElementPassed := false

	// Houd verwerkte parent IDs bij om duplicaten te voorkomen
	processedParentIDs := make(map[string]bool)

	for _, row := range rows {
		// Verwerk rijen zonder parent ID of met een overeenkomende parent ID
		if row.ParentID == parentID || parentID == "" {
			// Voorkom het meerdere keren verwerken van dezelfde parent ID
			if processedParentIDs[row.ParentID] {
				continue
			}
			processedParentIDs[row.ParentID] = true

			p.log.Debug().
				Str("rowID", row.ID).
				Str("rowParentID", row.ParentID).
				Msg("Processing slice row")

			valueElement := reflect.New(value.Type().Elem()).Elem()

			// Vul het element met de geünificeerde functie
			singleRow := []datasource.RowData{row}
			passed, err := p.populateStruct(structPath, valueElement, row.ParentID, singleRow, filter)
			if err != nil {
				p.log.Error().
					Err(err).
					Str("structPath", structPath).
					Msg("Error populating slice element")
				return false, fmt.Errorf("error populating slice element: %w", err)
			}

			if passed {
				anyElementPassed = true
			}

			allElements = reflect.Append(allElements, valueElement)
		}
	}

	value.Set(allElements)

	p.log.Debug().
		Bool("anyElementPassed", anyElementPassed).
		Int("totalElements", allElements.Len()).
		Msg("Slice population completed")

	return anyElementPassed, nil
}

// Geünificeerde functie die zowel structuren als geneste velden vult
func (p *ProcessorService) populateStruct(path string, value reflect.Value, parentID string,
	rows []datasource.RowData, filter []*types.Filter) (bool, error) {

	p.log.Debug().
		Str("path", path).
		Str("parentID", parentID).
		Int("rowCount", len(rows)).
		Msg("Populating struct")

	anyFieldPopulated := false
	processedRows := make(map[string]bool)

	// Proces alle relevante rijen
	for _, row := range rows {
		// Controleer of deze rij overeenkomt met onze criteria en nog niet is verwerkt
		if (row.ParentID == parentID || parentID == "") && !processedRows[row.ID] {
			p.log.Debug().Str("path", path).Str("row.ID", row.ID).Msg("Processing struct")
			processedRows[row.ID] = true

			// Eerst directe velden vullen
			structPassed, err := p.populateStructFields(path, value.Addr().Interface(), row, filter)
			if err != nil {
				return false, fmt.Errorf("failed to populate struct fields at %s: %w", path, err)
			}

			// Daarna geneste velden verwerken
			nestedPassed, err := p.populateNestedFields(path, value, row.ID, filter)
			if err != nil {
				return false, err
			}

			if structPassed || nestedPassed {
				anyFieldPopulated = true
			}
		}
	}

	return anyFieldPopulated, nil
}
func (p *ProcessorService) populateStructFields(structPath string, structPtr interface{}, row datasource.RowData, filter []*types.Filter) (bool, error) {
	structValue := reflect.ValueOf(structPtr).Elem()
	structType := structValue.Type()
	processedFields := make(map[string]bool)
	anyFieldPassed := false

	p.log.Debug().
		Str("path", structPath).
		Str("receivedParentID", row.ParentID).
		Msg("Starting to populate fields with parent ID")

	p.log.Debug().Str("structPath", structPath).Msg("Populating struct fields")

	// First process all Coding and CodeableConcept fields
	for i := 0; i < structType.NumField(); i++ {
		field := structValue.Field(i)
		fieldType := field.Type().String()
		fieldName := structType.Field(i).Name

		// Handle special FHIR types
		if strings.Contains(fieldType, "Coding") ||
			strings.Contains(fieldType, "CodeableConcept") ||
			strings.Contains(fieldType, "Quantity") {

			codingPath := fmt.Sprintf("%s.%s", structPath, strings.ToLower(fieldName))

			p.processedPaths[codingPath] = true

			codingRows, exists := p.result[codingPath]
			if !exists {
				continue
			}
			p.log.Debug().Str("codingPath", codingPath).Str("row.ID", row.ID).
				Msg("Processing Coding or Quantity field")
			// Handle CodeableConcept (single or slice)
			if strings.Contains(fieldType, "CodeableConcept") {
				err := p.setCodeableConceptField(field, codingPath, fieldName, row.ID, codingRows, processedFields)
				if err != nil {
					// Check if this field has a filter
					passed, err := true, error(nil)
					if err != nil {
						return false, err
					}
					if !passed {
						p.log.Debug().Err(err).Str("path", codingPath).Msg("CodeableConcept did not pass filter")
						continue
					}
				}
				anyFieldPassed = true
			} else {
				// Handle regular Coding and Quantity fields
				for _, codingRow := range codingRows {
					if codingRow.ParentID == row.ID {
						if strings.Contains(fieldType, "Quantity") {
							if err := p.setCodingOrQuantityFromRow(codingPath, codingPath, field, fieldName, codingRow, processedFields, false); err != nil {
								return false, err
							}
							anyFieldPassed = true
						}
						if strings.Contains(fieldType, "Coding") {
							if err := p.setCodingOrQuantityFromRow(codingPath, codingPath, field, fieldName, codingRow, processedFields, true); err != nil {
								return false, err
							}
							anyFieldPassed = true
						}
					}
				}

				// Check filter if exists
				passed, err := true, error(nil)
				if err != nil {
					return false, err
				}
				if !passed {
					p.log.Debug().
						Str("fieldPath", codingPath).
						Msg("Field did not pass filter")
					return false, nil
				}
			}
		}
	}

	// Then process regular fields
	for key, value := range row.Data {
		if processedFields[key] {
			continue
		}

		for i := 0; i < structType.NumField(); i++ {
			fieldName := structType.Field(i).Name
			if processedFields[fieldName] {
				continue
			}

			if strings.EqualFold(fieldName, key) {
				if err := p.setField(structPath, structPtr, fieldName, value); err != nil {
					return false, fmt.Errorf("failed to set field %s: %w", fieldName, err)
				}

				fieldPath := fmt.Sprintf("%s.%s", structPath, strings.ToLower(fieldName))
				passed, err := true, error(nil)
				if err != nil {
					return false, fmt.Errorf("failed to check filter for field %s: %w", fieldName, err)
				}
				if !passed {
					p.log.Debug().
						Str("fieldPath", fieldPath).
						Msg("Field did not pass filter")
					return false, nil
				}

				processedFields[fieldName] = true
				anyFieldPassed = true
				break
			}
		}
	}

	// Handle ID field if not already processed
	if idField := structValue.FieldByName("Id"); idField.IsValid() && idField.CanSet() && !processedFields["Id"] {
		p.log.Debug().Str("fieldName", "Id").Str("value", row.ID).Msg("Setting ID field")
		if err := p.setField(structPath, structPtr, "Id", row.ID); err != nil {
			return false, fmt.Errorf("failed to set Id field: %w", err)
		}
		anyFieldPassed = true
	}

	return anyFieldPassed, nil
}

// Add this debug helper function
func (p *ProcessorService) debugPrintProcessedPaths(location string) {
	p.log.Debug().Str("location", location).Msg("=== PROCESSED PATHS DEBUG ===")
	for path, processed := range p.processedPaths {
		p.log.Debug().Str("path", path).Bool("processed", processed).Msg("Path processing state")
	}
	p.log.Debug().Str("location", location).Msg("=== END PROCESSED PATHS DEBUG ===")
}

// populateNestedFields verwerkt geneste velden in een struct
func (p *ProcessorService) populateNestedFields(parentPath string, parentValue reflect.Value, parentID string, filter []*types.Filter) (bool, error) {
	p.log.Debug().
		Str("parentPath", parentPath).
		Str("parentID", parentID).
		Msg("Starting to populate nested fields")

	p.debugPrintProcessedPaths("Start of populateNestedFields for " + parentPath)

	anyFieldPassed := false

	for i := 0; i < parentValue.NumField(); i++ {
		field := parentValue.Field(i)
		fieldType := parentValue.Type().Field(i)
		fieldName := fieldType.Name

		// Skip unexported fields
		if fieldType.PkgPath != "" {
			continue
		}

		// IMPORTANT! Consistent field path construction
		// For FHIR, first letter should be lowercase
		var fieldPath string
		if len(fieldName) > 0 {
			fieldPath = fmt.Sprintf("%s.%s", parentPath, strings.ToLower(fieldName[0:1])+fieldName[1:])
		} else {
			fieldPath = fmt.Sprintf("%s.%s", parentPath, strings.ToLower(fieldName))
		}

		// Debug field we're checking
		p.log.Debug().
			Str("fieldPath", fieldPath).
			Str("fieldName", fieldName).
			Str("fieldType", field.Type().String()).
			Bool("alreadyProcessed", p.processedPaths[fieldPath]).
			Msg("Checking nested field")

		// Skip if we've already processed this path
		if p.processedPaths[fieldPath] {
			p.log.Debug().
				Str("fieldPath", fieldPath).
				Msg("⚠️ SKIPPING already processed nested field")
			continue
		}

		// Check if there's data for this path
		rows, exists := p.result[fieldPath]
		if !exists {
			p.log.Debug().
				Str("fieldPath", fieldPath).
				Msg("No data for this field path")
			continue
		}

		p.log.Debug().
			Str("fieldPath", fieldPath).
			Int("rowCount", len(rows)).
			Msg("Found data for field path, about to process")

		// Determine effective parent ID
		effectiveParentID := parentID
		if len(rows) > 0 && rows[0].ParentID == "" {
			effectiveParentID = ""
			p.log.Debug().
				Str("fieldPath", fieldPath).
				Msg("Using empty parentID because first row has empty parentID")
		}

		// Populate the field
		passed, err := p.determinePopulateType(fieldPath, field, effectiveParentID, filter)
		if err != nil {
			p.log.Error().
				Err(err).
				Str("fieldPath", fieldPath).
				Msg("❌ Error populating nested field")
			return false, err
		}

		// CORRECT: Mark as processed AFTER successful processing
		p.processedPaths[fieldPath] = true
		p.log.Debug().
			Str("fieldPath", fieldPath).
			Msg("✅ Marked path as processed AFTER successful population")

		if passed {
			anyFieldPassed = true
			p.log.Debug().
				Str("fieldPath", fieldPath).
				Msg("Field passed filters")
		}
	}

	p.debugPrintProcessedPaths("End of populateNestedFields for " + parentPath)
	return anyFieldPassed, nil
}

// setBasicType handles basic type field population
func (p *ProcessorService) setBasicType(path string, field reflect.Value, parentID string, rows []datasource.RowData, filter []*types.Filter) (bool, error) {
	p.log.Debug().Str("path", path).Msg("Setting basic type")
	for _, row := range rows {
		if row.ParentID == parentID || parentID == "" {
			for key, value := range row.Data {
				p.log.Debug().Str("key", key).Interface("value", value).Msg("Setting field")
				if err := p.setField(path, field.Addr().Interface(), key, value); err != nil {
					p.log.Error().Err(err).Str("key", key).Msg("Failed to set field")
					return false, err
				}

				// Check filter if exists
				passed, err := true, error(nil)
				if err != nil {
					p.log.Error().Err(err).Msg("Filter check failed")
					return false, err
				}
				p.log.Debug().Bool("passed", passed).Msg("Field passed filter check")
				return passed, nil
			}
		}
	}
	return true, nil
}

func (rp *ProcessorService) setField(structPath string, structPtr interface{}, fieldName string, value interface{}) error {
	fhirPath := fmt.Sprintf("%s.%s", structPath, strings.ToLower(fieldName[:1])+fieldName[1:])

	structValue := reflect.ValueOf(structPtr)
	if structValue.Kind() != reflect.Ptr || structValue.IsNil() {
		return fmt.Errorf("structPtr must be a non-nil pointer to struct")
	}

	structElem := structValue.Elem()
	field := structElem.FieldByName(fieldName)
	if !field.IsValid() || !field.CanSet() {
		return fmt.Errorf("invalid or cannot set field: %s", fieldName)
	}

	if value == nil {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}

	// Handle pointer types first - initialize if needed
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem() // Dereference for further processing
	}

	rp.log.Debug().Str("structPath", structPath).Str("fieldName", fieldName).Str("fieldType", field.Type().String()).Interface("value", value).Msg("Setting field")

	// Now check for special types after potentially dereferencing
	switch field.Type().String() {
	case "fhir.Date":
		return rp.setDateField(field, value)
	case "json.Number":
		return rp.setJSONNumber(field, value)
	}

	// Check for code types that might need concept mapping
	structFieldType := field.Type()
	if typeHasCodeMethod(structFieldType) { // Suggesting it is a code type
		rp.log.Debug().Msgf("The type has a Code() method, indicating a 'code' type.")

		// Retrieve the specific concept map from the repository
		bindingValueSet, err := rp.structDefSvc.GetBindingValueSet(fhirPath)
		if err != nil {
			rp.log.Error().Err(err).Msg("Failed to get ValueSet")
		}

		rp.log.Debug().Msgf("binding ValueSet: %s", bindingValueSet)

		conceptMapURL, err := rp.conceptMapSvc.GetConceptMapsByValuesetURL(bindingValueSet)
		if err != nil {
			rp.log.Error().Err(err).Msg("Failed to get ConceptMap")
		}

		rp.log.Debug().Msgf("conceptMapURL: %s", conceptMapURL)

		// Perform concept mapping using the retrieved concept map
		translatedCode, err := rp.conceptMapSvc.TranslateCode(conceptMapURL, value.(string), true)
		if err != nil {
			rp.log.Error().Err(err).Msg("Failed to translate code")
		} else {
			if translatedCode != nil {
				rp.log.Debug().Msgf("Translated code: %s", translatedCode.TargetCode)
				value = translatedCode.TargetCode
			} else {
				rp.log.Debug().Msg("No translation found")
			}
		}
	}

	// Check if type implements UnmarshalJSON
	if unmarshaler, ok := field.Addr().Interface().(json.Unmarshaler); ok {
		rp.log.Debug().Str("field", field.Type().String()).Msg("Setting field with UnmarshalJSON")
		var jsonBytes []byte
		var err error

		switch v := value.(type) {
		case string:
			jsonBytes = []byte(`"` + v + `"`)
		case []byte:
			jsonBytes = v
		default:
			if jsonBytes, err = json.Marshal(value); err != nil {
				return fmt.Errorf("failed to marshal value: %w", err)
			}
		}

		if err := unmarshaler.UnmarshalJSON(jsonBytes); err != nil {
			return fmt.Errorf("failed to unmarshal value for type %s: %w", field.Type().String(), err)
		}
		return nil
	}

	// Handle basic types
	return rp.setBasicField(field, value)
}

// Sets a FHIR Date field
func (rp *ProcessorService) setDateField(field reflect.Value, value interface{}) error {
	rp.log.Debug().Str("field", field.Type().String()).Msg("Setting date field")
	// Ensure we can take the address of the field
	if !field.CanAddr() {
		return fmt.Errorf("cannot take address of date field")
	}

	dateStr := ""
	switch v := value.(type) {
	case string:
		dateStr = v
	case []uint8:
		dateStr = string(v)
	default:
		return fmt.Errorf("cannot convert %T to Date", value)
	}

	// Get the Date object we can unmarshal into
	date := field.Addr().Interface().(*fhir.Date)
	if err := date.UnmarshalJSON([]byte(`"` + dateStr + `"`)); err != nil {
		return fmt.Errorf("failed to parse date: %w", err)
	}

	return nil
}

// Sets a json.Number field
func (rp *ProcessorService) setJSONNumber(field reflect.Value, value interface{}) error {
	var num json.Number
	switch v := value.(type) {
	case json.Number:
		num = v
	case string:
		num = json.Number(v)
	case float64:
		num = json.Number(strconv.FormatFloat(v, 'f', -1, 64))
	case int64:
		num = json.Number(strconv.FormatInt(v, 10))
	case []uint8:
		num = json.Number(string(v))
	default:
		return fmt.Errorf("cannot convert %T to json.Number", value)
	}

	field.Set(reflect.ValueOf(num))
	return nil
}

// Helper function to check if type has Code method
func typeHasCodeMethod(t reflect.Type) bool {
	_, ok := t.MethodByName("Code")
	return ok
}

// Sets a basic field (standard types)
func (rp *ProcessorService) setBasicField(field reflect.Value, value interface{}) error {
	v := reflect.ValueOf(value)
	if field.Type() == v.Type() {
		field.Set(v)
		return nil
	}

	// Handle type conversions
	switch field.Kind() {
	case reflect.String:
		field.SetString(fmt.Sprint(value))
	case reflect.Bool:
		boolVal, err := strconv.ParseBool(fmt.Sprint(value))
		if err != nil {
			return err
		}
		field.SetBool(boolVal)
	case reflect.Int, reflect.Int64:
		intVal, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(intVal)
	case reflect.Float64:
		floatVal, err := strconv.ParseFloat(fmt.Sprint(value), 64)
		if err != nil {
			return err
		}
		field.SetFloat(floatVal)
	default:
		return fmt.Errorf("unsupported field type: %v", field.Kind())
	}
	return nil
}

// setCodeableConceptField handles setting CodeableConcept fields
func (p *ProcessorService) setCodeableConceptField(field reflect.Value, path string, fieldName string,
	parentID string, rows []datasource.RowData, processedFields map[string]bool) error {

	p.log.Debug().
		Str("path", path).
		Str("fieldName", fieldName).
		Str("parentID", parentID).
		Int("rowCount", len(rows)).
		Msg("Setting CodeableConcept field")

	// Determine if this is a slice or a single field
	isSlice := field.Kind() == reflect.Slice

	// Path for coding entries
	codingPath := fmt.Sprintf("%s.coding", path)

	// Get all coding rows for this path
	codingRows, exists := p.result[codingPath]
	if !exists {
		p.log.Debug().
			Str("codingPath", codingPath).
			Msg("No coding rows found for path")
	}

	// Mark both paths as processed
	p.processedPaths[path] = true
	p.processedPaths[codingPath] = true

	if isSlice {
		// Handle array of CodeableConcepts
		p.log.Debug().
			Str("fieldName", fieldName).
			Msg("Processing CodeableConcept as slice")

		if field.IsNil() {
			field.Set(reflect.MakeSlice(field.Type(), 0, len(rows)))
		}

		// Collect unique concepts
		conceptMap := make(map[string]datasource.RowData)
		for _, row := range rows {
			// For slice we look at top-level concepts
			if row.ParentID == parentID || parentID == "" {
				conceptMap[row.ID] = row
			}
		}

		// Process each unique concept
		for conceptID, conceptRow := range conceptMap {
			// Create new concept and populate it
			newConcept := reflect.New(field.Type().Elem()).Elem()

			// KEY FIX: Use the conceptID to find its codings
			if err := p.populateCodeableConcept(newConcept, path, conceptRow, conceptID, codingRows, processedFields); err != nil {
				return fmt.Errorf("failed to populate concept %s: %w", conceptID, err)
			}

			// Add to slice
			field.Set(reflect.Append(field, newConcept))
		}
	} else {
		// Handle single CodeableConcept
		if field.Kind() == reflect.Ptr {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			field = field.Elem()
		}

		// Find a row with matching parentID
		var conceptRow datasource.RowData
		var conceptID string

		for _, row := range rows {
			if row.ParentID == parentID || parentID == "" {
				conceptRow = row
				conceptID = row.ID // KEY FIX: Store the concept's ID
				p.log.Debug().
					Str("conceptID", conceptID).
					Msg("Found concept row with matching parentID")
				break
			}
		}

		// If no concept found but we have rows, use the first one
		if conceptID == "" && len(rows) > 0 {
			conceptRow = rows[0]
			conceptID = conceptRow.ID // KEY FIX: Get the ID even for default case
			p.log.Debug().
				Str("conceptID", conceptID).
				Msg("Using first available concept row")
		}

		// KEY FIX: Use the concept's ID to find its codings
		if err := p.populateCodeableConcept(field, path, conceptRow, conceptID, codingRows, processedFields); err != nil {
			return err
		}
	}

	return nil
}

// populateCodeableConcept fills a CodeableConcept with its data and codings
func (p *ProcessorService) populateCodeableConcept(concept reflect.Value, path string,
	conceptRow datasource.RowData, conceptID string, codingRows []datasource.RowData,
	processedFields map[string]bool) error {

	p.log.Debug().
		Str("path", path).
		Str("conceptID", conceptID).
		Interface("conceptData", conceptRow.Data).
		Msg("Populating CodeableConcept")

	// Set Text field if present
	if textValue, exists := conceptRow.Data["text"]; exists {
		textField := concept.FieldByName("Text")
		if textField.IsValid() && textField.CanSet() {
			if textField.Kind() == reflect.Ptr {
				if textField.IsNil() {
					textField.Set(reflect.New(textField.Type().Elem()))
				}
				textField.Elem().SetString(fmt.Sprint(textValue))
			} else {
				textField.SetString(fmt.Sprint(textValue))
			}
		}
	}

	// Get Coding field
	codingField := concept.FieldByName("Coding")
	if !codingField.IsValid() {
		return fmt.Errorf("invalid Coding field in CodeableConcept")
	}

	// Initialize Coding slice
	if codingField.Kind() == reflect.Slice && codingField.IsNil() {
		codingField.Set(reflect.MakeSlice(codingField.Type(), 0, 5))
	}

	// KEY FIX: Process all coding rows where ParentID matches this concept's ID
	if len(codingRows) > 0 && conceptID != "" {
		codingCount := 0

		for _, codingRow := range codingRows {
			// KEY FIX: Match by concept ID, not the parent ID from higher level
			if codingRow.ParentID == conceptID {
				p.log.Debug().
					Str("codingID", codingRow.ID).
					Str("codingParentID", codingRow.ParentID).
					Str("conceptID", conceptID).
					Msg("Adding coding to concept")

				if err := p.setCodingOrQuantityFromRow(path, path+".coding", codingField,
					"Coding", codingRow, processedFields, true); err != nil {
					return fmt.Errorf("failed to set coding: %w", err)
				}

				codingCount++
			}
		}

		p.log.Debug().
			Str("conceptID", conceptID).
			Int("codingCount", codingCount).
			Msg("Added codings to concept")
	} else {
		p.log.Debug().
			Str("conceptID", conceptID).
			Msg("No codings found for concept")
	}

	return nil
}

// Functie voor het vullen van Coding en Quantity velden
func (p *ProcessorService) setCodingOrQuantityFromRow(valuesetBindingPath string, structPath string,
	field reflect.Value, fieldName string, row datasource.RowData,
	processedFields map[string]bool, isCoding bool) error {

	// Haal veldwaarden op
	fieldValues := p.extractFieldValues(row, processedFields)

	var newValue interface{}
	code := fieldValues["code"]
	system := fieldValues["system"]

	if isCoding {
		// Coding verwerken
		if code == "" && system == "" {
			return nil // Sla over als er geen code of systeem is
		}

		// Maak Coding object
		coding := fhir.Coding{
			Code:    stringPtr(code),
			Display: stringPtr(fieldValues["display"]),
			System:  stringPtr(system),
		}

		// Concept mapping kan hier plaatsvinden (uitgecommentarieerd in originele code)
		// if mappedCode, _, err := p.conceptMapSvc.MapConcept(valuesetBindingPath, code); err == nil && mappedCode != "" {
		//     coding.Code = &mappedCode
		// }

		// Zet de waarde als pointer of direct object
		if field.Type().Kind() == reflect.Ptr {
			newValue = &coding
		} else {
			newValue = coding
		}
	} else {
		// Quantity verwerken
		quantity := fhir.Quantity{
			Value:  jsonNumberPtr(fieldValues["value"]),
			Unit:   stringPtr(fieldValues["unit"]),
			System: stringPtr(system),
			Code:   stringPtr(code),
		}

		// Concept mapping voor quantity codes (uitgecommentarieerd in originele code)
		// if code != "" {
		//     if mappedCode, _, err := p.conceptMapSvc.MapConcept(valuesetBindingPath, code); err == nil && mappedCode != "" {
		//         quantity.Code = &mappedCode
		//     }
		// }

		// Zet de waarde als pointer of direct object
		if field.Type().Kind() == reflect.Ptr {
			newValue = &quantity
		} else {
			newValue = quantity
		}
	}

	// Toevoegen aan slice of direct veld instellen
	if field.Kind() == reflect.Slice {
		if field.IsNil() {
			field.Set(reflect.MakeSlice(field.Type(), 0, 1))
		}

		// Controleer op duplicaten
		exists := false
		for i := 0; i < field.Len(); i++ {
			if isDuplicate(field.Index(i).Interface(), newValue) {
				exists = true
				break
			}
		}

		// Alleen toevoegen als het geen duplicaat is
		if !exists {
			field.Set(reflect.Append(field, reflect.ValueOf(newValue)))
		}
	} else {
		// Direct veld instellen
		field.Set(reflect.ValueOf(newValue))
	}

	return nil
}

// Helper functie om veldwaarden uit row.Data te extraheren
func (p *ProcessorService) extractFieldValues(row datasource.RowData, processedFields map[string]bool) map[string]string {
	fieldValues := make(map[string]string)

	for key, value := range row.Data {
		keyLower := strings.ToLower(key)
		strValue := fmt.Sprint(value)

		// Converteer byte slice naar string indien nodig
		if byteSlice, ok := value.([]byte); ok {
			strValue = string(byteSlice)
		}

		// Determine which field this is based on key suffix
		switch {
		case strings.HasSuffix(keyLower, "code"):
			fieldValues["code"] = strValue
			processedFields[key] = true
		case strings.HasSuffix(keyLower, "display"):
			fieldValues["display"] = strValue
			processedFields[key] = true
		case strings.HasSuffix(keyLower, "system"):
			fieldValues["system"] = strValue
			processedFields[key] = true
		case strings.HasSuffix(keyLower, "unit"):
			fieldValues["unit"] = strValue
			processedFields[key] = true
		case strings.HasSuffix(keyLower, "value"):
			fieldValues["value"] = strValue
			processedFields[key] = true
		}
	}

	return fieldValues
}

// Helper functie om te controleren op duplicate coding of quantity
func isDuplicate(existing, newValue interface{}) bool {
	switch e := existing.(type) {
	case fhir.Coding:
		if n, ok := newValue.(fhir.Coding); ok {
			return e.Code != nil && n.Code != nil && *e.Code == *n.Code &&
				e.System != nil && n.System != nil && *e.System == *n.System
		}
	case fhir.Quantity:
		if n, ok := newValue.(fhir.Quantity); ok {
			return e.Code != nil && n.Code != nil && *e.Code == *n.Code &&
				e.System != nil && n.System != nil && *e.System == *n.System
		}
	}
	return false
}

// Helper functie om een json.Number pointer te maken als de waarde niet leeg is
func jsonNumberPtr(s string) *json.Number {
	if s == "" {
		return nil
	}
	num := json.Number(s)
	return &num
}

// Helper functie om een string pointer te maken als de waarde niet leeg is
func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
