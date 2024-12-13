package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/SanteonNL/fenix/cmd/fenix/datasource"
	"github.com/SanteonNL/fenix/models/fhir"
)

func (p *ProcessorService) populateAndFilter(ctx context.Context, resource interface{}, result datasource.ResourceResult, filter *Filter) (bool, error) {
	value := reflect.ValueOf(resource).Elem()
	typeName := value.Type().Name()

	fmt.Printf("Starting to populate and filter: %s\n", typeName)

	return p.populateAndFilterValue(ctx, value, typeName, result, filter, true)
}

func (p *ProcessorService) populateAndFilterValue(ctx context.Context, value reflect.Value, path string, result datasource.ResourceResult, filter *Filter, isTopLevel bool) (bool, error) {
	// Track if any field passes the filter for slice cases
	anyFieldPassedFilter := false

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		fieldName := value.Type().Field(i).Name
		fieldPath := path
		if isTopLevel {
			fieldPath = fmt.Sprintf("%s.%s", path, strings.ToLower(fieldName))

			// Only track processed paths at top level
			if _, processed := p.processedPaths.Load(fieldPath); processed {
				continue
			}
			p.processedPaths.Store(fieldPath, true)
		}

		rows, exists := result[fieldPath]
		if !exists {
			continue
		}

		// Check if this field has a filter
		var searchType string
		var err error
		if filter != nil {
			fmt.Printf("Checking filter for field: %s\n", fieldPath)
			searchType, err = p.pathInfoSvc.GetSearchTypeByCode(fieldPath, filter.Code)
		}

		if filter != nil {
			fmt.Printf("Applying filter on field: %s with search type: %s", fieldPath, searchType)
		}

		// Populate and filter based on field type
		passed, err := p.populateAndFilterField(ctx, field, fieldPath, rows, filter, searchType)
		if err != nil {
			return false, err
		}

		// For slice types, we track if any element passed
		if field.Kind() == reflect.Slice {
			anyFieldPassedFilter = anyFieldPassedFilter || passed
		} else if !passed {
			// For non-slice types, fail immediately if filter fails
			return false, nil
		}
	}

	// For slice types, at least one element must have passed
	if filter != nil && value.Kind() == reflect.Slice {
		return anyFieldPassedFilter, nil
	}

	return true, nil
}

func (p *ProcessorService) populateAndFilterField(ctx context.Context, field reflect.Value, path string, rows []datasource.RowData, filter *Filter, searchType string) (bool, error) {
	if !field.CanSet() {
		return true, nil
	}

	// Handle special FHIR types first
	switch field.Type().String() {
	case "fhir.CodeableConcept", "*fhir.CodeableConcept":
		if err := p.setCodeableConceptField(field, path, rows); err != nil {
			return false, err
		}
		if filter != nil && searchType != "" {
			return p.checkFilter(ctx, field, path, searchType, filter)
		}
		return true, nil

	case "fhir.Coding", "*fhir.Coding":
		if err := p.setCodingField(field, path, rows); err != nil {
			return false, err
		}
		if filter != nil && searchType != "" {
			return p.checkFilter(ctx, field, path, searchType, filter)
		}
		return true, nil
	}

	switch field.Kind() {
	case reflect.Ptr:
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return p.populateAndFilterField(ctx, field.Elem(), path, rows, filter, searchType)

	case reflect.Slice:
		return p.populateAndFilterSlice(ctx, field, path, rows, filter, searchType)

	case reflect.Struct:
		return p.populateAndFilterStruct(ctx, field, path, rows, filter, searchType)

	default:
		passed, err := p.populateBasicField(field, rows)
		if err != nil || !passed {
			return false, err
		}

		if filter != nil && searchType != "" {
			return p.checkFilter(ctx, field, path, searchType, filter)
		}
		return true, nil
	}
}

func (p *ProcessorService) populateAndFilterSlice(ctx context.Context, field reflect.Value, path string, rows []datasource.RowData, filter *Filter, searchType string) (bool, error) {
	sliceType := field.Type()
	newSlice := reflect.MakeSlice(sliceType, 0, len(rows))
	anyPassed := false

	for _, row := range rows {
		elem := reflect.New(sliceType.Elem()).Elem()
		passed, err := p.populateAndFilterField(ctx, elem, path, []datasource.RowData{row}, filter, searchType)
		if err != nil {
			return false, err
		}

		if passed {
			newSlice = reflect.Append(newSlice, elem)
			anyPassed = true
		}
	}

	if anyPassed {
		field.Set(newSlice)
	}
	return anyPassed, nil
}

func (p *ProcessorService) populateAndFilterStruct(ctx context.Context, field reflect.Value, path string, rows []datasource.RowData, filter *Filter, searchType string) (bool, error) {
	if len(rows) == 0 {
		return true, nil
	}

	row := rows[0]
	allPassed := true

	for key, value := range row.Data {
		upperKey := strings.ToUpper(key[:1]) + key[1:]
		structField := field.FieldByName(upperKey)

		if !structField.IsValid() || !structField.CanSet() {
			continue
		}

		if err := p.setField(ctx, structField, value); err != nil {
			return false, err
		}

		// Check if this nested field has a filter
		if filter != nil && searchType != "" {
			nestedPath := fmt.Sprintf("%s.%s", path, strings.ToLower(key))
			passed, err := p.checkFilter(ctx, structField, nestedPath, searchType, filter)
			if err != nil {
				return false, err
			}
			allPassed = allPassed && passed
		}
	}

	return allPassed, nil
}

func (p *ProcessorService) populateBasicField(field reflect.Value, rows []datasource.RowData) (bool, error) {
	if len(rows) == 0 {
		return true, nil
	}

	row := rows[0]
	for _, value := range row.Data {
		if err := p.setField(context.Background(), field, value); err != nil {
			return false, err
		}
		return true, nil
	}

	return true, nil
}

// Helper functions for setting field values

func (p *ProcessorService) setField(ctx context.Context, field reflect.Value, value interface{}) error {
	if !field.CanSet() {
		return nil
	}

	if value == nil {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}

	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem()
	}

	switch field.Type().String() {
	case "fhir.Date":
		return p.setDateField(field, value)
	case "json.Number":
		return p.setJSONNumber(field, value)
	}

	if typeHasCodeMethod(field.Type()) {
		return p.setCodeField(field, value)
	}

	return p.setBasicField(field, value)
}

func (p *ProcessorService) setCodeField(field reflect.Value, value interface{}) error {
	var strValue string
	switch v := value.(type) {
	case string:
		strValue = v
	case []uint8:
		strValue = string(v)
	case int, int64, float64:
		return fmt.Errorf("numeric value %v not supported for code field %s", v, field.Type().Name())
	default:
		return fmt.Errorf("unsupported type for code value: %T", value)
	}

	codePtr := reflect.New(field.Type()).Interface()

	jsonValue, err := json.Marshal(strValue)
	if err != nil {
		return fmt.Errorf("failed to marshal code string: %w", err)
	}

	if unmarshaler, ok := codePtr.(json.Unmarshaler); ok {
		if err := unmarshaler.UnmarshalJSON(jsonValue); err != nil {
			return fmt.Errorf("failed to unmarshal code value '%s': %w", strValue, err)
		}
		field.Set(reflect.ValueOf(codePtr).Elem())
		return nil
	}

	return fmt.Errorf("code type %s does not implement UnmarshalJSON", field.Type().Name())
}

func (p *ProcessorService) setDateField(field reflect.Value, value interface{}) error {
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

	date := field.Addr().Interface().(*fhir.Date)
	if err := date.UnmarshalJSON([]byte(`"` + dateStr + `"`)); err != nil {
		return fmt.Errorf("failed to parse date: %w", err)
	}

	return nil
}

func (p *ProcessorService) setJSONNumber(field reflect.Value, value interface{}) error {
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

func (p *ProcessorService) setBasicField(field reflect.Value, value interface{}) error {
	v := reflect.ValueOf(value)

	if field.Type() == v.Type() {
		field.Set(v)
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(fmt.Sprint(value))
	case reflect.Bool:
		bVal, err := strconv.ParseBool(fmt.Sprint(value))
		if err != nil {
			return err
		}
		field.SetBool(bVal)
	case reflect.Int, reflect.Int64:
		iVal, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(iVal)
	case reflect.Float64:
		fVal, err := strconv.ParseFloat(fmt.Sprint(value), 64)
		if err != nil {
			return err
		}
		field.SetFloat(fVal)
	default:
		return fmt.Errorf("unsupported field type: %v", field.Kind())
	}

	return nil
}

func (p *ProcessorService) setCodeableConceptField(field reflect.Value, path string, rows []datasource.RowData) error {
	// Initialize field if needed
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem()
	}

	concept := field.Addr().Interface().(*fhir.CodeableConcept)

	// Group rows by their concept ID
	conceptRows := make(map[string][]datasource.RowData)
	for _, row := range rows {
		conceptID := row.ID
		if row.ParentID != "" {
			conceptID = row.ParentID
		}
		conceptRows[conceptID] = append(conceptRows[conceptID], row)
	}

	// Process each group of rows
	for _, rowGroup := range conceptRows {
		// Process text field
		for _, row := range rowGroup {
			if textValue, ok := row.Data["text"]; ok {
				concept.Text = stringPtr(fmt.Sprint(textValue))
				break
			}
		}

		// Initialize Coding slice if needed
		if concept.Coding == nil {
			concept.Coding = make([]fhir.Coding, 0)
		}

		// Process coding data
		for _, row := range rowGroup {
			coding, err := p.createCodingFromRow(row, path)
			if err != nil {
				return err
			}
			if coding != nil && !p.hasDuplicateCoding(concept.Coding, *coding) {
				concept.Coding = append(concept.Coding, *coding)
			}
		}
	}

	return nil
}

func (p *ProcessorService) setCodingField(field reflect.Value, path string, rows []datasource.RowData) error {
	// Handle single Coding vs slice of Codings
	if field.Kind() == reflect.Slice {
		// Initialize slice if needed
		if field.IsNil() {
			field.Set(reflect.MakeSlice(field.Type(), 0, len(rows)))
		}

		// Process each row into a Coding
		for _, row := range rows {
			coding, err := p.createCodingFromRow(row, path)
			if err != nil {
				return err
			}
			if coding != nil {
				// Convert to the right type (pointer or value)
				var codingValue reflect.Value
				if field.Type().Elem().Kind() == reflect.Ptr {
					codingValue = reflect.ValueOf(coding)
				} else {
					codingValue = reflect.ValueOf(*coding)
				}
				field.Set(reflect.Append(field, codingValue))
			}
		}
	} else {
		// Handle single Coding
		if len(rows) > 0 {
			coding, err := p.createCodingFromRow(rows[0], path)
			if err != nil {
				return err
			}
			if coding != nil {
				if field.Kind() == reflect.Ptr {
					if field.IsNil() {
						field.Set(reflect.New(field.Type().Elem()))
					}
					field.Elem().Set(reflect.ValueOf(*coding))
				} else {
					field.Set(reflect.ValueOf(*coding))
				}
			}
		}
	}

	return nil
}

func (p *ProcessorService) createCodingFromRow(row datasource.RowData, path string) (*fhir.Coding, error) {
	var code, system, display string

	// Extract values from row data
	for key, value := range row.Data {
		strValue := fmt.Sprint(value)
		switch {
		case strings.HasSuffix(strings.ToLower(key), "code"):
			code = strValue
		case strings.HasSuffix(strings.ToLower(key), "system"):
			system = strValue
		case strings.HasSuffix(strings.ToLower(key), "display"):
			display = strValue
		}
	}

	// Skip if no code or system
	if code == "" && system == "" {
		return nil, nil
	}

	// // Perform concept mapping if needed
	// if mappedCode, err := p.pathInfoSvc.GetMappedCode(path, code); err == nil && mappedCode != "" {
	// 	code = mappedCode
	// }

	return &fhir.Coding{
		Code:    stringPtr(code),
		System:  stringPtr(system),
		Display: stringPtr(display),
	}, nil
}

func (p *ProcessorService) hasDuplicateCoding(existingCodings []fhir.Coding, newCoding fhir.Coding) bool {
	for _, existing := range existingCodings {
		if (existing.Code == nil && newCoding.Code == nil ||
			existing.Code != nil && newCoding.Code != nil && *existing.Code == *newCoding.Code) &&
			(existing.System == nil && newCoding.System == nil ||
				existing.System != nil && newCoding.System != nil && *existing.System == *newCoding.System) {
			return true
		}
	}
	return false
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Helper function to check if type has Code method
func typeHasCodeMethod(t reflect.Type) bool {
	_, ok := t.MethodByName("Code")
	return ok
}
