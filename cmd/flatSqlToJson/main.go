package main

import (
	"encoding/json"
	"fmt"

	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/SanteonNL/fenix/models/fhir"
	"github.com/SanteonNL/fenix/util"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
)

// ConceptMapperMap is a nested map structure for efficient lookups
// The structure is: fhirPath -> sourceSystem -> sourceCode -> TargetCode
type ConceptMapperMap map[string]map[string]map[string]TargetCode

// TargetCode represents the mapped code in the target system
type TargetCode struct {
	System  string
	Code    string
	Display string
}

var globalConceptMaps ConceptMapperMap

// FHIRResourceFactory is a function type that creates a new instance of a FHIR resource
type FHIRResourceFactory func() interface{}

var FHIRResourceMap = map[string]FHIRResourceFactory{
	"Patient":     func() interface{} { return &fhir.Patient{} },
	"Observation": func() interface{} { return &fhir.Observation{} },
}

func initializeGenderConceptMap() {
	globalConceptMaps = ConceptMapperMap{
		"Patient.gender": {
			"http://hl7.org/fhir/administrative-gender": {
				"male": TargetCode{
					System:  "http://snomed.info/sct",
					Code:    "248153007",
					Display: "Male",
				},
				"female": TargetCode{
					System:  "http://snomed.info/sct",
					Code:    "248152002",
					Display: "Female",
				},
				"other": TargetCode{
					System:  "http://snomed.info/sct",
					Code:    "394743007",
					Display: "Other",
				},
				"unknown": TargetCode{
					System:  "http://snomed.info/sct",
					Code:    "unknown",
					Display: "Unknown",
				},
			},
			"": { // For system-agnostic mappings
				"M": TargetCode{
					System:  "http://hl7.org/fhir/administrative-gender",
					Code:    "male",
					Display: "Male",
				},
				"F": TargetCode{
					System:  "http://hl7.org/fhir/administrative-gender",
					Code:    "female",
					Display: "Female",
				},
				"O": TargetCode{
					System:  "http://hl7.org/fhir/administrative-gender",
					Code:    "other",
					Display: "Other",
				},
				"U": TargetCode{
					System:  "http://hl7.org/fhir/administrative-gender",
					Code:    "unknown",
					Display: "Unknown",
				},
			},
		},
	}
}

func main() {
	startTime := time.Now()
	log := zerolog.New(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) { w.Out = os.Stdout })).With().Timestamp().Caller().Logger()
	log.Debug().Msg("Starting fenix")

	db, err := sqlx.Connect("postgres", "postgres://postgres:mysecretpassword@localhost:5432/tsl_employee?sslmode=disable")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to the database")
	}
	defer db.Close()

	// Set up data source
	queryPath := util.GetAbsolutePath("queries/hix/flat/patient.sql")
	queryBytes, err := os.ReadFile(queryPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read query file")
	}
	query := string(queryBytes)
	dataSource := NewSQLDataSource(db, query, log)

	// Set up search parameters
	searchParameterMap := SearchParameterMap{
		"Patient.identifier": SearchParameter{
			Code:  "identifier",
			Type:  "token",
			Value: "https://santeon.nl|123456",
		},
	}

	// Initialize concept maps
	initializeGenderConceptMap()

	resourceName := "Patient"                    // Example: processing Patients
	resourceIDs := []string{"123", "456", "789"} // Example patient numbers
	resources, err := ProcessMultipleFHIRResources(dataSource, resourceName, resourceIDs, searchParameterMap, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to process FHIR resources")
	}

	for i, resource := range resources {
		jsonData, err := json.MarshalIndent(resource, "", "  ")
		if err != nil {
			log.Error().Err(err).Msgf("Failed to marshal resource %d to JSON", i)
			continue
		}
		fmt.Printf("%s %d data:\n", resourceName, i+1)
		fmt.Println(string(jsonData))
		fmt.Println()
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	log.Debug().Msgf("Execution time: %s", duration)
}

func ProcessMultipleFHIRResources(ds DataSource, resourceName string, resourceIDs []string, searchParameterMap SearchParameterMap, log zerolog.Logger) ([]interface{}, error) {
	factory, ok := FHIRResourceMap[resourceName]
	if !ok {
		return nil, fmt.Errorf("unsupported FHIR resource: %s", resourceName)
	}

	var originalQuery string

	var resources []interface{}
	if sqlDS, ok := ds.(*SQLDataSource); ok {
		originalQuery = sqlDS.query
	}

	for _, id := range resourceIDs {
		resource := factory()
		v := reflect.ValueOf(resource).Elem()

		identifier := "'" + id + "'"

		if sqlDS, ok := ds.(*SQLDataSource); ok {
			sqlDS.query = strings.ReplaceAll(originalQuery, ":identifier", identifier)
			log.Debug().Str("query", originalQuery).Str("id", id).Msg("Modified query")
		}

		data, err := ds.Read()
		if err != nil {
			return nil, fmt.Errorf("error reading data for %s %s: %w", resourceName, id, err)
		}

		filterResult, err := populateStruct(v, data, "", "", searchParameterMap, log)
		if err != nil {
			return nil, fmt.Errorf("error populating %s %s: %w", resourceName, id, err)
		}

		if filterResult.Passed {
			resources = append(resources, resource)
		} else {
			log.Info().Str("id", id).Msg(filterResult.Message)
		}
	}

	return resources, nil
}

func populateStruct(field reflect.Value, resultMap map[string][]map[string]interface{}, fhirPath string, parentID string, searchParameterMap SearchParameterMap, log zerolog.Logger) (*FilterResult, error) {
	if fhirPath == "" {
		fhirPath = field.Type().Name()
	}

	filterResult, err := populateField(field, resultMap, fhirPath, parentID, searchParameterMap, log)
	if err != nil {
		return nil, err
	}
	if !filterResult.Passed {
		return filterResult, nil
	}

	return &FilterResult{Passed: true}, nil
}

func populateField(field reflect.Value, resultMap map[string][]map[string]interface{}, fhirPath string, parentID string, searchParameterMap SearchParameterMap, log zerolog.Logger) (*FilterResult, error) {
	log.Debug().Str("field", fhirPath).Msg("Populating field")
	rows, exists := resultMap[fhirPath]
	if !exists {
		log.Debug().Msgf("No data found for field: %s", fhirPath)
		return &FilterResult{Passed: true}, nil
	}

	switch field.Kind() {
	case reflect.Slice:
		return populateSlice(field, rows, fhirPath, parentID, resultMap, searchParameterMap, log)
	case reflect.Struct:
		return populateStructFields(field, rows, fhirPath, parentID, resultMap, searchParameterMap, log)
	case reflect.Ptr:
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return populateField(field.Elem(), resultMap, fhirPath, parentID, searchParameterMap, log)
	default:
		return populateBasicType(field, rows, fhirPath, parentID, searchParameterMap, log)
	}
}

func populateSlice(field reflect.Value, rows []map[string]interface{}, fieldName string, parentID string, resultMap map[string][]map[string]interface{}, searchParameterMap SearchParameterMap, log zerolog.Logger) (*FilterResult, error) {
	anyElementPassed := false
	for _, row := range rows {
		if row["parent_id"] == parentID || parentID == "" {
			elem := reflect.New(field.Type().Elem()).Elem()
			if err := populateElement(elem, row, fieldName, resultMap, searchParameterMap, log); err != nil {
				return nil, err
			}

			filterResult, err := applyFilter(elem, fieldName, searchParameterMap, log)
			if err != nil {
				return nil, err
			}

			if filterResult.Passed {
				anyElementPassed = true
				log.Debug().
					Str("field", fieldName).
					Msg("Slice element passed filter, continuing slice population")
			}

			// Always add the element to the slice, regardless of filter result
			field.Set(reflect.Append(field, elem))
		}
	}

	if anyElementPassed {
		return &FilterResult{Passed: true}, nil
	}

	return &FilterResult{Passed: false, Message: fmt.Sprintf("No elements in slice passed filter: %s", fieldName)}, nil
}

func populateStructFields(field reflect.Value, rows []map[string]interface{}, fieldName string, parentID string, resultMap map[string][]map[string]interface{}, searchParameterMap SearchParameterMap, log zerolog.Logger) (*FilterResult, error) {
	for _, row := range rows {
		if row["parent_id"] == parentID || parentID == "" {
			if err := populateElement(field, row, fieldName, resultMap, searchParameterMap, log); err != nil {
				return nil, err
			}

			filterResult, err := applyFilter(field, fieldName, searchParameterMap, log)
			if err != nil {
				return nil, err
			}
			if !filterResult.Passed {
				return filterResult, nil
			}

			break // We only need one matching row for struct fields
		}
	}
	return &FilterResult{Passed: true}, nil
}

func populateElement(elem reflect.Value, row map[string]interface{}, fieldName string, resultMap map[string][]map[string]interface{}, searchParameterMap SearchParameterMap, log zerolog.Logger) error {
	if err := populateStructFromRow(elem.Addr().Interface(), row, log); err != nil {
		return err
	}

	currentID, _ := row["id"].(string)
	return populateNestedElements(elem, resultMap, fieldName, currentID, searchParameterMap, log)
}

func populateNestedElements(parentField reflect.Value, resultMap map[string][]map[string]interface{}, parentPath string, parentID string, searchParameterMap SearchParameterMap, log zerolog.Logger) error {
	for i := 0; i < parentField.NumField(); i++ {
		childField := parentField.Field(i)
		childName := parentField.Type().Field(i).Name
		childPath := fmt.Sprintf("%s.%s", parentPath, strings.ToLower(childName))

		if hasDataForPath(resultMap, childPath) {
			filterResult, err := populateField(childField, resultMap, childPath, parentID, searchParameterMap, log)
			if err != nil {
				return err
			}
			if !filterResult.Passed {
				return fmt.Errorf(filterResult.Message)
			}
		}
	}
	return nil
}
func populateBasicType(field reflect.Value, rows []map[string]interface{}, fieldName string, parentID string, searchParameterMap SearchParameterMap, log zerolog.Logger) (*FilterResult, error) {
	for _, row := range rows {
		if row["parent_id"] == parentID || parentID == "" {
			for key, value := range row {
				if strings.EqualFold(key, fieldName) {
					if err := SetField(field.Addr().Interface(), fieldName, value, log); err != nil {
						return nil, err
					}
					return applyFilter(field, fieldName, searchParameterMap, log)
				}
			}
		}
	}
	return &FilterResult{Passed: true}, nil
}

func populateStructFromRow(obj interface{}, row map[string]interface{}, log zerolog.Logger) error {
	v := reflect.ValueOf(obj).Elem()
	t := v.Type()

	for key, value := range row {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			fieldNameLower := strings.ToLower(field.Name)

			if fieldNameLower == strings.ToLower(key) {
				err := SetField(obj, field.Name, value, log)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func SetField(obj interface{}, name string, value interface{}, log zerolog.Logger) error {

	structValue := reflect.ValueOf(obj)
	if structValue.Kind() != reflect.Ptr || structValue.IsNil() {
		return fmt.Errorf("obj must be a non-nil pointer to a struct")
	}

	structElem := structValue.Elem()
	if structElem.Kind() != reflect.Struct {
		return fmt.Errorf("obj must point to a struct")
	}

	field := structElem.FieldByName(name)
	if !field.IsValid() {
		return fmt.Errorf("no such field: %s in obj", name)
	}

	if !field.CanSet() {
		return fmt.Errorf("cannot set field %s", name)
	}

	if value == nil {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}

	// Check if the field is a pointer to a type that implements UnmarshalJSON
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		unmarshalJSONMethod := field.MethodByName("UnmarshalJSON")
		if unmarshalJSONMethod.IsValid() {
			//Map value first
			stringValue := getStringValue(reflect.ValueOf(value))
			TargetCode, err := mapConceptCode(stringValue, "Patient.gender", log)
			if err != nil {
				return fmt.Errorf("failed to map concept code: %v", err)
			}
			value = TargetCode.Code
			// Convert the value to JSON
			jsonValue, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to marshal value to JSON: %v", err)
			}

			// Call UnmarshalJSON
			results := unmarshalJSONMethod.Call([]reflect.Value{reflect.ValueOf(jsonValue)})
			if len(results) > 0 && !results[0].IsNil() {
				return results[0].Interface().(error)
			}
			return nil
		}
	}

	fieldValue := reflect.ValueOf(value)

	// Handle conversion from []uint8 to []string if needed
	if field.Type() == reflect.TypeOf([]string{}) && fieldValue.Type() == reflect.TypeOf([]uint8{}) {
		var strSlice []string
		if err := json.Unmarshal(value.([]uint8), &strSlice); err != nil {
			return fmt.Errorf("failed to unmarshal []uint8 to []string: %v", err)
		}
		field.Set(reflect.ValueOf(strSlice))
		return nil
	}

	if field.Kind() == reflect.Ptr && (field.Type().Elem().Kind() == reflect.String || field.Type().Elem().Kind() == reflect.Bool) {
		var newValue reflect.Value

		switch field.Type().Elem().Kind() {
		case reflect.String:
			var strValue string
			switch v := value.(type) {
			case string:
				strValue = v
			case int, int8, int16, int32, int64:
				strValue = fmt.Sprintf("%d", v)
			case uint, uint8, uint16, uint32, uint64:
				strValue = fmt.Sprintf("%d", v)
			case float32, float64:
				strValue = fmt.Sprintf("%f", v)
			case bool:
				strValue = strconv.FormatBool(v)
			case time.Time:
				strValue = v.Format(time.RFC3339)
			default:
				return fmt.Errorf("cannot convert %T to *string", value)
			}
			newValue = reflect.ValueOf(&strValue)
		case reflect.Bool:
			var boolValue bool
			switch v := value.(type) {
			case bool:
				boolValue = v
			case string:
				var err error
				boolValue, err = strconv.ParseBool(v)
				if err != nil {
					return fmt.Errorf("cannot convert string to *bool: %s", v)
				}
			default:
				return fmt.Errorf("cannot convert %T to *bool", value)
			}
			newValue = reflect.ValueOf(&boolValue)
		}

		field.Set(newValue)
	} else {
		if field.Type() != fieldValue.Type() {
			return fmt.Errorf("provided value type didn't match obj field type %s for field %s and %s ", field.Type(), name, fieldValue.Type())
		}

		field.Set(fieldValue)
	}

	return nil
}
func applyFilter(field reflect.Value, fhirPath string, searchParameterMap SearchParameterMap, log zerolog.Logger) (*FilterResult, error) {
	searchParameter, ok := searchParameterMap[fhirPath]
	if !ok {
		log.Debug().
			Str("field", fhirPath).
			Msg("No filter found for fhirPath")
		return &FilterResult{Passed: true, Message: fmt.Sprintf("No filter defined for: %s", fhirPath)}, nil
	}

	if field.Kind() == reflect.Slice {
		// For slices, we delegate to populateSlice which now handles the filtering
		return &FilterResult{Passed: true}, nil
	}

	filterResult, err := FilterField(field, searchParameter, fhirPath, log)
	if err != nil {
		return nil, err
	}
	log.Debug().
		Str("field", fhirPath).
		Bool("passed", filterResult.Passed).
		Msg("Apply filter result")
	if !filterResult.Passed {
		return &FilterResult{Passed: false, Message: fmt.Sprintf("Field filtered out: %s", fhirPath)}, nil
	}

	return &FilterResult{Passed: true}, nil
}

func hasDataForPath(resultMap map[string][]map[string]interface{}, path string) bool {
	if _, exists := resultMap[path]; exists {
		return true
	}

	return false
}

func extendFhirPath(parentPath, childName string) string {
	return fmt.Sprintf("%s.%s", parentPath, strings.ToLower(childName))
}

func mapConceptCode(value string, fhirPath string, log zerolog.Logger) (TargetCode, error) {
	// Simple implementation without system handling
	log.Debug().Str("fhirPath", fhirPath).Str("sourceCode", value).Msg("Mapping concept code")
	if conceptMap, ok := globalConceptMaps[fhirPath]; ok {
		if systemMap, ok := conceptMap[""]; ok {
			if targetCode, ok := systemMap[value]; ok {
				log.Debug().
					Str("fhirPath", fhirPath).
					Str("sourceCode", value).
					Str("targetCode", targetCode.Code).
					Msg("Applied concept mapping")
				return targetCode, nil
			}
		}
	}

	// If no mapping found, return the original value
	return TargetCode{Code: value}, nil
}
