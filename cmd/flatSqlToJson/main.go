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
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type SearchParameter struct {
	FieldName string
	Value     interface{}
}

type SearchFilter struct {
	Code       string   `json:"code"`
	Modifier   []string `json:"modifier,omitempty"`
	Comparator string   `json:"comparator,omitempty"`
	Value      string   `json:"value"`
	Type       string   `json:"type,omitempty"`
	Expression string
}

// Key = Patient.identifier
type SearchFilterGroup map[string]SearchFilter

func main() {
	startTime := time.Now()

	l := zerolog.New(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) { w.Out = os.Stdout })).With().Timestamp().Caller().Logger()

	l.Debug().Msg("Starting flatSqlToJson")

	db, err := sqlx.Connect("postgres", "postgres://postgres:mysecretpassword@localhost:5432/tsl_employee?sslmode=disable")
	if err != nil {
		l.Fatal().Err(err).Msg("Failed to connect to the database")
	}

	queryPath := util.GetAbsolutePath("queries/hix/flat/patient.sql")

	queryBytes, err := os.ReadFile(queryPath)
	if err != nil {
		l.Fatal().Msgf("Failed to read query file: %s", queryPath)
	}
	query := string(queryBytes)

	SQLDataSource := NewSQLDataSource(db, query)

	mapperPath := util.GetAbsolutePath("config/csv_mappings.json")
	mapper, err := LoadCSVMapperFromConfig(mapperPath)
	if err != nil {
		l.Fatal().Err(err).Msg("Failed to load mapper configuration")
	}

	csvDataSource := NewCSVDataSource("test/data/sim/patient.csv", mapper)

	// Choose which DataSource to use
	var dataSource DataSource
	useCSV := false // Set this to false to use SQL
	if useCSV {
		dataSource = csvDataSource
	} else {
		dataSource = SQLDataSource
	}

	searchFilterGroup := SearchFilterGroup{"Patient.identifier": SearchFilter{Code: "identifier", Type: "token", Value: "https://santeon.nl|123", Expression: "Patient.identifier"},
		"Patient.birthdate": SearchFilter{Code: "birthdate", Type: "date", Comparator: "ge", Value: "20060101", Expression: "Patient.birthdate"}}

	patient := fhir.Patient{}

	err = ExtractAndMapData(dataSource, &patient, searchFilterGroup, l)
	if err != nil {
		l.Fatal().Stack().Err(errors.WithStack(err)).Msg("Failed to populate struct")
		return
	}

	jsonData, err := json.MarshalIndent(patient, "", "  ")
	if err != nil {
		l.Fatal().Err(err).Msg("Failed to marshal patient to JSON")
		return
	}

	fmt.Println("JSON data:")
	fmt.Println(string(jsonData))

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	l.Debug().Msgf("Execution time: %s", duration)
}

func ExtractAndMapData(ds DataSource, s interface{}, sg SearchFilterGroup, logger zerolog.Logger) error {
	data, err := ds.Read()
	if err != nil {
		return err
	}
	logger.Debug().Interface("rawData", data).Msgf("Data before mapping:\n%+v", data)

	v := reflect.ValueOf(s).Elem()
	return populateAndFilterStruct(v, data, "", sg)
}

func fieldMatchesIdentifierFilter(field reflect.Value, system, code string) bool {
	if field.Kind() == reflect.Slice {
		for i := 0; i < field.Len(); i++ {
			if matchesIdentifierFilter(field.Index(i), system, code) {
				return true
			}
		}
	} else if field.Kind() == reflect.Struct {
		return matchesIdentifierFilter(field, system, code)
	}
	return false
}

func matchesIdentifierFilter(v reflect.Value, system, code string) bool {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return false
	}

	identifierField := v.FieldByName("Identifier")
	if identifierField.IsValid() && identifierField.Kind() == reflect.Slice {
		for i := 0; i < identifierField.Len(); i++ {
			coding := identifierField.Index(i)
			if matchesIndentifierValues(coding, system, code) {
				return true
			}
		}
	} else {
		return matchesIndentifierValues(v, system, code)
	}

	return false
}
func matchesIndentifierValues(v reflect.Value, system, code string) bool {
	systemField := v.FieldByName("System")
	codeField := v.FieldByName("Value")

	if !systemField.IsValid() || !codeField.IsValid() {
		return false
	}

	if systemField.Kind() == reflect.Ptr {
		if systemField.IsNil() {
			return false
		}
		systemField = systemField.Elem()
	}

	if codeField.Kind() == reflect.Ptr {
		if codeField.IsNil() {
			return false
		}
		codeField = codeField.Elem()
	}

	log.Debug().Str("system", systemField.String()).Str("code", codeField.String()).Msg("Matching identifier values")

	return (system == "" || systemField.String() == system) && codeField.String() == code
}

func parseFilter(filter string) (string, string) {
	parts := strings.Split(filter, "|")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}

func fieldExistsInResultMap(resultMap map[string][]map[string]interface{}, fieldName string) bool {
	fieldName = strings.ToLower(fieldName)
	fieldName = strings.ToUpper(string(fieldName[0])) + fieldName[1:]

	if _, ok := resultMap[fieldName]; ok {
		return true
	}

	for key := range resultMap {
		if strings.HasPrefix(key, fieldName+".") {
			log.Debug().Msgf("Nested field exists in resultMap: %s", key)
			return true
		}
	}
	return false
}

func populateAndFilterStruct(v reflect.Value, resultMap map[string][]map[string]interface{}, parentField string, sg SearchFilterGroup) error {
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("value is not a struct")
	}

	structName := v.Type().Name()
	fullFieldName := parentField

	if parentField == "" {
		fullFieldName = structName
	}

	if data, ok := resultMap[fullFieldName]; ok {
		for _, row := range data {
			err := populateStructFromRow(v.Addr().Interface(), row)
			if err != nil {
				return err
			}
		}
	}

	structID := ""
	if idField := v.FieldByName("Id"); idField.IsValid() && idField.Kind() == reflect.Ptr && idField.Type().Elem().Kind() == reflect.String {
		if idField.Elem().IsValid() {
			structID = idField.Elem().String()
		}
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldName := v.Type().Field(i).Name
		fullFieldName := fullFieldName + "." + fieldName
		fullFieldName = strings.ToLower(fullFieldName)
		fullFieldName = strings.ToUpper(string(fullFieldName[0])) + fullFieldName[1:]

		log.Debug().Str("field", fullFieldName).Msg("Processing field")

		var resultMapKey string
		if parentField == "" {
			resultMapKey = structName
		} else {
			resultMapKey = fullFieldName
		}

		if fieldExistsInResultMap(resultMap, resultMapKey) {
			err := populateField(field, resultMap, fullFieldName, structID, sg)
			if err != nil {
				return err
			}

			if _, ok := sg[fullFieldName]; ok {
				log.Debug().Str("field", fullFieldName).Msg("Field exists in search filter group")
			}

			err = FilterField(field, sg, fullFieldName)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func populateField(field reflect.Value, resultMap map[string][]map[string]interface{}, fieldName string, parentID string, sg SearchFilterGroup) error {
	log.Debug().Msgf("Populating field %s in populateField: %s and parentID", fieldName, parentID)
	switch field.Kind() {
	case reflect.Slice:
		return populateSlice(field, resultMap, fieldName, parentID, sg)
	case reflect.Struct:
		return populateAndFilterStruct(field, resultMap, fieldName, sg)
	case reflect.Ptr:
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return populateField(field.Elem(), resultMap, fieldName, parentID, sg)
	default:
		return populateBasicType(field, resultMap, fieldName)
	}
}

func populateBasicType(field reflect.Value, resultMap map[string][]map[string]interface{}, fullFieldName string) error {
	log.Debug().Msgf("Populating basic type fullFieldName: %s", fullFieldName)

	data, ok := resultMap[fullFieldName]
	if !ok {
		return nil // Don't set any value if the field is not in the resultMap
	}

	fieldName := strings.Split(fullFieldName, ".")[len(strings.Split(fullFieldName, "."))-1]
	log.Debug().Msgf("Populating basic type fieldName: %s", fieldName)

	for _, row := range data {
		for key, value := range row {
			if strings.EqualFold(key, fieldName) {
				log.Debug().Msgf("Setting field: %s with value: %v", fieldName, value)
				return SetField(field.Addr().Interface(), fieldName, value)
			}
		}
	}
	return nil
}

func populateSlice(field reflect.Value, resultMap map[string][]map[string]interface{}, fieldName string, parentID string, sg SearchFilterGroup) error {
	log.Debug().Msgf("Populating slice field: %s with parentID: %s", fieldName, parentID)
	if data, ok := resultMap[fieldName]; ok {
		for _, row := range data {
			if row["parent_id"] == parentID || parentID == "" {
				newElem := reflect.New(field.Type().Elem()).Elem()
				err := populateStructFromRow(newElem.Addr().Interface(), row)
				if err != nil {
					return err
				}

				newElemID := ""
				if idField := newElem.FieldByName("Id"); idField.IsValid() && idField.Kind() == reflect.Ptr && idField.Type().Elem().Kind() == reflect.String {
					if idField.Elem().IsValid() {
						newElemID = idField.Elem().String()
					}
				}

				for i := 0; i < newElem.NumField(); i++ {
					nestedField := newElem.Field(i)
					nestedFieldName := newElem.Type().Field(i).Name
					nestedFullFieldName := fieldName + "." + nestedFieldName
					nestedFullFieldName = strings.ToLower(nestedFullFieldName)
					nestedFullFieldName = strings.ToUpper(string(nestedFullFieldName[0])) + nestedFullFieldName[1:]

					if fieldExistsInResultMap(resultMap, nestedFullFieldName) {
						err := populateField(nestedField, resultMap, nestedFullFieldName, newElemID, sg)
						if err != nil {
							return err
						}
					}
				}

				field.Set(reflect.Append(field, newElem))
			}
		}
	} else {
		log.Debug().Msgf("No data found for field: %s", fieldName)
	}
	return nil
}

func populateStructFromRow(obj interface{}, row map[string]interface{}) error {
	v := reflect.ValueOf(obj).Elem()
	t := v.Type()

	for key, value := range row {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			fieldNameLower := strings.ToLower(field.Name)

			if fieldNameLower == strings.ToLower(key) {
				err := SetField(obj, field.Name, value)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func SetField(obj interface{}, name string, value interface{}) error {

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
