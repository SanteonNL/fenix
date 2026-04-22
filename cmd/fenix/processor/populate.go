package processor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/SanteonNL/fenix/cmd/fenix/datasource"
	"github.com/SanteonNL/fenix/cmd/fenix/types"
	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// Add this type at the top level
type ProcessedPaths map[string]bool

// populateResourceStruct maintains your current population logic
func (p *ProcessorService) populateResourceStruct(value reflect.Value, filter []*types.Filter) (bool, error) {
	return p.determinePopulateType(p.resourceType, value, "", filter)
}

// determinePopulateType handles different field types
func (p *ProcessorService) determinePopulateType(structPath string, value reflect.Value, parentID string, filter []*types.Filter) (bool, error) {
	//p.log.Debug().Str("structPath", structPath).Str("value.Kind()", value.Kind().String()).Msg("Determining populate type")
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

// Modify populateSlice to mark processed paths
func (p *ProcessorService) populateSlice(structPath string, value reflect.Value, parentID string, rows []datasource.RowData, filter []*types.Filter) (bool, error) {
	p.log.Debug().
		Str("structPath", structPath).
		Str("parentID", parentID).
		Int("total_rows", len(rows)).
		Msg("Populating slice")

	// Mark this path as processed
	p.processedPaths[structPath] = true

	// Create slice to hold all elements
	allElements := reflect.MakeSlice(value.Type(), 0, len(rows))
	anyElementPassed := false

	// Track processed parent IDs to avoid duplicates
	processedParentIDs := make(map[string]bool)

	for _, row := range rows {
		// Process rows without a parent ID or with a matching parent ID
		// Importantly, allow multiple entries with different parent IDs
		if row.ParentID == parentID || parentID == "" {
			// Prevent processing the same parent ID multiple times
			if processedParentIDs[row.ParentID] {
				continue
			}
			processedParentIDs[row.ParentID] = true

			p.log.Debug().
				Str("rowID", row.ID).
				Str("rowParentID", row.ParentID).
				Msg("Processing slice row")

			valueElement := reflect.New(value.Type().Elem()).Elem()

			// Populate the element
			passed, err := p.populateStructAndNestedFields(structPath, valueElement, row, filter)
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

			// Always add element to the slice
			allElements = reflect.Append(allElements, valueElement)
		}
	}

	// Set the complete slice with all elements
	value.Set(allElements)

	p.log.Debug().
		Bool("anyElementPassed", anyElementPassed).
		Int("totalElements", allElements.Len()).
		Msg("Slice population completed")

	return anyElementPassed, nil
}

// populateStruct handles struct population with filter integration
func (p *ProcessorService) populateStruct(path string, value reflect.Value, parentID string, rows []datasource.RowData, filter []*types.Filter) (bool, error) {
	p.log.Debug().
		Str("path", path).
		Str("parentID", parentID).
		Int("rowCount", len(rows)).
		Msg("Populating struct")
	anyFieldPopulated := false
	processedRows := make(map[string]bool)

	// Process each row that matches the parent ID
	for _, row := range rows {
		if (row.ParentID == parentID || parentID == "") && !processedRows[row.ID] {
			p.log.Debug().Str("path", path).Str("row.ID", row.ID).Msg("Processing struct")
			processedRows[row.ID] = true

			// First populate direct fields
			structPassed, err := p.populateStructFields(path, value.Addr().Interface(), row, filter)
			if err != nil {
				return false, fmt.Errorf("failed to populate struct fields at %s: %w", path, err)
			}

			// Then handle nested fields
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

// Part 1: Struct and Nested Fields
// populateStructAndNestedFields handles both direct and nested field population
func (p *ProcessorService) populateStructAndNestedFields(structPath string, value reflect.Value, row datasource.RowData, filter []*types.Filter) (bool, error) {
	// First populate and filter struct fields
	structPassed, err := p.populateStructFields(structPath, value.Addr().Interface(), row, filter)
	if err != nil {
		return false, fmt.Errorf("failed to populate struct fields at %s: %w", structPath, err)
	}

	if !structPassed {
		return false, nil
	}

	// Then handle nested fields
	return p.populateNestedFields(structPath, value, row.ID, filter)
}

// Modify populateNestedFields to check processed paths
// populateNestedFields handles nested field population
func (p *ProcessorService) populateNestedFields(parentPath string, parentValue reflect.Value, parentID string, filter []*types.Filter) (bool, error) {
	anyFieldPassed := false

	for i := 0; i < parentValue.NumField(); i++ {
		field := parentValue.Field(i)
		fieldName := parentValue.Type().Field(i).Name
		fieldPath := fmt.Sprintf("%s.%s", parentPath, strings.ToLower(fieldName))

		// Skip if we've already processed this path
		if p.processedPaths[fieldPath] {
			p.log.Debug().
				Str("fieldPath", fieldPath).
				Msg("Skipping already processed nested field")
			continue
		}

		if rows, exists := p.result[fieldPath]; exists {
			p.processedPaths[fieldPath] = true
			p.log.Debug().Str("fieldPath", fieldPath).Msg("Marked path as processed in populateNestedFields")

			// Important change: For top-level nested fields, use empty parentID if the data shows empty parentID
			effectiveParentID := parentID
			if len(rows) > 0 && rows[0].ParentID == "" {
				effectiveParentID = ""
			}

			passed, err := p.determinePopulateType(fieldPath, field, effectiveParentID, filter)
			if err != nil {
				return false, err
			}
			if passed {
				anyFieldPassed = true
			}
		}
	}

	return anyFieldPassed, nil
}

func (p *ProcessorService) populateStructFields(structPath string, structPtr interface{}, row datasource.RowData, filter []*types.Filter) (bool, error) {
	structValue := reflect.ValueOf(structPtr).Elem()
	structType := structValue.Type()
	processedFields := make(map[string]bool)
	anyFieldPassed := false

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
			p.log.Debug().Str("codingPath", codingPath).Msg("Processing Coding or Quantity field")
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

func (p *ProcessorService) setCodeableConceptField(field reflect.Value, path string, fieldName string, parentID string, rows []datasource.RowData, processedFields map[string]bool) error {
	p.log.Debug().
		Str("path", path).
		Str("fieldName", fieldName).
		Str("parentID", parentID).
		Int("rowCount", len(rows)).
		Msg("Setting CodeableConcept field")

	isSlice := field.Kind() == reflect.Slice

	if isSlice {
		p.log.Debug().
			Str("fieldName", fieldName).
			Msg("Processing CodeableConcept as slice")

		if field.IsNil() {
			field.Set(reflect.MakeSlice(field.Type(), 0, len(rows)))
		}

		// Create a map to track unique concepts
		conceptMap := make(map[string]datasource.RowData)

		// First, collect all unique top-level concepts
		for _, row := range rows {
			if row.ParentID == "" { // Only process top-level concepts
				conceptMap[row.ID] = row
			}
		}

		// Process each unique concept
		for conceptID, conceptRow := range conceptMap {
			newConcept := reflect.New(field.Type().Elem()).Elem()
			if err := p.populateCodeableConcept(newConcept, path, conceptRow, processedFields); err != nil {
				return fmt.Errorf("failed to populate concept %s: %w", conceptID, err)
			}
			field.Set(reflect.Append(field, newConcept))
		}
	} else {
		// Non-slice case
		if field.Kind() == reflect.Ptr {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			field = field.Elem()
		}

		// Use empty concept if one exists
		var conceptRow datasource.RowData
		for _, row := range rows {
			if (row.ParentID == parentID || parentID == "") && row.ParentID == "" {
				conceptRow = row
				break
			}
		}

		// Always process even if conceptRow is empty, using ID "1" if no row found
		if conceptRow.ID == "" {
			conceptRow = datasource.RowData{ID: "1"}
		}

		if err := p.populateCodeableConcept(field, path, conceptRow, processedFields); err != nil {
			return err
		}
	}

	return nil
}

func (p *ProcessorService) populateCodeableConcept(conceptValue reflect.Value, path string, row datasource.RowData, processedFields map[string]bool) error {
	p.log.Debug().
		Str("path", path).
		Str("rowID", row.ID).
		Interface("rowData", row.Data).
		Msg("Populating CodeableConcept")

	// Get the Coding field
	codingField := conceptValue.FieldByName("Coding")
	if !codingField.IsValid() {
		return fmt.Errorf("invalid Coding field in CodeableConcept")
	}

	// Initialize Coding slice
	if codingField.Kind() == reflect.Slice && codingField.IsNil() {
		codingField.Set(reflect.MakeSlice(codingField.Type(), 0, 1))
	}

	// Look up coding rows
	codingPath := fmt.Sprintf("%s.coding", path)
	codingRows, exists := p.result[codingPath]

	if exists {
		// Simply process all coding rows that match the parent ID
		for _, codingRow := range codingRows {
			if codingRow.ParentID == row.ID {
				if err := p.setCodingOrQuantityFromRow(path, codingPath, codingField, "Coding", codingRow, processedFields, true); err != nil {
					return fmt.Errorf("failed to set coding: %w", err)
				}
			}
		}
	}

	// Process text field if present
	if textValue, exists := row.Data["text"]; exists {
		textField := conceptValue.FieldByName("Text")
		if textField.IsValid() && textField.CanSet() && textField.Kind() == reflect.Ptr {
			if textField.IsNil() {
				textField.Set(reflect.New(textField.Type().Elem()))
			}
			textField.Elem().SetString(fmt.Sprint(textValue))
		}
	}

	return nil
}
func (p *ProcessorService) setCodingOrQuantityFromRow(valuesetBindingPath string, structPath string, field reflect.Value, fieldName string, row datasource.RowData, processedFields map[string]bool, isCoding bool) error {
	fieldValues := p.extractFieldValues(row, processedFields)

	var newValue interface{}
	code := fieldValues["code"]
	system := fieldValues["system"]

	if isCoding {
		if code == "" && system == "" {
			return nil
		}
		coding := fhir.Coding{
			Code:    stringPtr(code),
			Display: stringPtr(fieldValues["display"]),
			System:  stringPtr(system),
		}

		// // Handle concept mapping
		// if mappedCode, _, err := p.conceptMapSvc.MapConcept(valuesetBindingPath, code); err == nil && mappedCode != "" {
		// 	coding.Code = &mappedCode
		// }

		if field.Type().Kind() == reflect.Ptr {
			newValue = &coding
		} else {
			newValue = coding
		}
	} else {
		quantity := fhir.Quantity{
			Value:  jsonNumberPtr(fieldValues["value"]),
			Unit:   stringPtr(fieldValues["unit"]),
			System: stringPtr(system),
			Code:   stringPtr(code),
		}

		// if code != "" {
		// 	if mappedCode, _, err := p.conceptMapSvc.MapConcept(valuesetBindingPath, code); err == nil && mappedCode != "" {
		// 		quantity.Code = &mappedCode
		// 	}
		// }

		if field.Type().Kind() == reflect.Ptr {
			newValue = &quantity
		} else {
			newValue = quantity
		}
	}

	if field.Kind() == reflect.Slice {
		if field.IsNil() {
			field.Set(reflect.MakeSlice(field.Type(), 0, 1))
		}

		exists := false
		for i := 0; i < field.Len(); i++ {
			if isDuplicate(field.Index(i).Interface(), newValue) {
				exists = true
				break
			}
		}

		if !exists {
			field.Set(reflect.Append(field, reflect.ValueOf(newValue)))
		}
	} else {
		field.Set(reflect.ValueOf(newValue))
	}

	return nil
}

// extractFieldValues extracts key values from row data based on suffixes
func (rp *ProcessorService) extractFieldValues(row datasource.RowData, processedFields map[string]bool) map[string]string {
	fieldValues := make(map[string]string)

	for key, value := range row.Data {
		keyLower := strings.ToLower(key)
		strValue := fmt.Sprint(value)

		// Handle byte slice to string conversion if needed
		if byteSlice, ok := value.([]byte); ok {
			strValue = string(byteSlice)
		}

		switch {
		case strings.HasSuffix(keyLower, "code"):
			fieldValues["code"] = strValue
			processedFields[key] = true
			rp.log.Debug().Str("code", strValue).Msg("Found code")
		case strings.HasSuffix(keyLower, "display"):
			fieldValues["display"] = strValue
			processedFields[key] = true
			rp.log.Debug().Str("display", strValue).Msg("Found display")
		case strings.HasSuffix(keyLower, "system"):
			fieldValues["system"] = strValue
			processedFields[key] = true
			rp.log.Debug().Str("system", strValue).Msg("Found system")
		case strings.HasSuffix(keyLower, "unit"):
			fieldValues["unit"] = strValue
			processedFields[key] = true
			rp.log.Debug().Str("unit", strValue).Msg("Found unit")
		case strings.HasSuffix(keyLower, "value"):
			fieldValues["value"] = strValue
			processedFields[key] = true
			rp.log.Debug().Str("value", strValue).Msg("Found value")
		}
	}
	return fieldValues
}

// setCodingOrQuantityField sets a single field or appends to a slice if needed
func (rp *ProcessorService) setCodingOrQuantityField(field reflect.Value, newValue interface{}, code string, system string) {
	// If the field is expecting a pointer, ensure newVal is a pointer as well

	if field.Kind() == reflect.Slice {
		var newSlice reflect.Value
		if field.IsNil() {
			newSlice = reflect.MakeSlice(field.Type(), 0, 1)
		} else {
			newSlice = field
		}

		// Check for duplicates
		exists := false
		for i := 0; i < newSlice.Len(); i++ {
			existing := newSlice.Index(i).Interface()
			if isDuplicate(existing, newValue) {
				exists = true
				break
			}
		}

		if !exists {
			newSlice = reflect.Append(newSlice, reflect.ValueOf(newValue))
			field.Set(newSlice)
			rp.log.Debug().
				Str("code", code).
				Str("system", system).
				Msg("Added new entry to slice")
		}
	} else {
		field.Set(reflect.ValueOf(newValue))
		rp.log.Debug().
			Str("code", code).
			Str("system", system).
			Msg("Set single field entry")
	}
}

// Helper to check for duplicate coding or quantity.
// As slice elements are not pointers, only Coding and Quantity are checked
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

// Helper function to set a json.Number pointer if the value is not empty
func jsonNumberPtr(s string) *json.Number {
	if s == "" {
		return nil
	}
	num := json.Number(s)
	return &num
}

// Helper function to set a string pointer if the value is not empty
func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Helper function to check if a field name matches a pattern with a suffix
func fieldMatchesPattern(fieldName string, prefix string, suffix string) bool {
	fieldLower := strings.ToLower(fieldName)
	return strings.HasPrefix(fieldLower, strings.ToLower(prefix)) &&
		strings.HasSuffix(fieldLower, strings.ToLower(suffix))
}

// Part 2: Field Setting and Type Conversion
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

	// TODO: this can be removed but for now keep it for reference
	// Cannot use field.Type as above because codes have different types, e.g. ObservationStatus has type ObservationStatus but
	// for other resource it can be other types. So we need to check if the type has a Code() method
	// equivalents in SetField function in
	// https://github.com/SanteonNL/fenix/blob/feature/lw_add_conceptmapping_based_on_renamed_functions_before_rewrite_tommy/cmd/flatSqlToJson/main.go
	//structFieldName := fieldName
	//structValueElement := structElem
	//structField := field
	//inputValue := value

	// Perform concept mapping for codes if applicable
	structFieldType := field.Type()
	if typeHasCodeMethod(structFieldType) { // Suggesting it is a code type
		rp.log.Debug().Msgf("The type has a Code() method, indicating a 'code' type.")

		// Retrieve the specific concept map from the repository
		// TODO: make it more generic
		// TODO: make sure that nils etc. are handled properly
		// TODO: also translate the display field
		// TODO: make a function insetad of much code within setFied
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

// Helper function to check for nested data
func hasDataForPath(resultMap map[string][]datasource.RowData, path string) bool {
	if _, exists := resultMap[path]; exists {
		return true
	}
	return false
}

// Helper function to get byte value
func getByteValue(v interface{}) ([]byte, error) {
	switch value := v.(type) {
	case string:
		return []byte(value), nil
	case []byte:
		return value, nil
	default:
		return json.Marshal(v)
	}
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

// Helper function to check if type has Code method
func typeHasCodeMethod(t reflect.Type) bool {
	_, ok := t.MethodByName("Code")
	return ok
}

func (p *ProcessorService) debugPrintResultMap() {
	p.log.Debug().Msg("START: Full Result Map Contents")
	for path, rows := range p.result {
		p.log.Debug().
			Str("path", path).
			Int("row_count", len(rows)).
			Msg("Result Map Path")

		for _, row := range rows {
			rowJSON, _ := json.MarshalIndent(row, "", "  ")
			p.log.Debug().
				Str("path", path).
				Str("row_id", row.ID).
				Str("parent_id", row.ParentID).
				RawJSON("row_data", rowJSON).
				Msg("Row Details")
		}
	}
	p.log.Debug().Msg("END: Full Result Map Contents")
}
