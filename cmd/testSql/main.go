package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	fhir "github.com/SanteonNL/fenix/models/fhir/r4"
	"github.com/SanteonNL/fenix/util"
	"github.com/gorilla/mux"

	_ "github.com/jinzhu/gorm/dialects/postgres"
)

type Endpoints struct {
	ResourceType string     `json:"resourceType"`
	SQLFile      string     `json:"sqlFile"`
	Endpoint     []Endpoint `json:"endpoint"`
}
type SearchFilter struct {
	Code       string   `json:"code"`
	Modifier   []string `json:"modifier,omitempty"`
	Comparator string   `json:"comparator,omitempty"`
	Value      string   `json:"value"`
	Type       string   `json:"type,omitempty"`
}
type Endpoint struct {
	SearchParameter []SearchFilter `json:"searchParameter"`
	SQLFile         string         `json:"sqlFile"`
}

func main() {

	r := mux.NewRouter()
	r.HandleFunc("/patients", GetAllPatients).Methods("GET")
	log.Println("Server started on port 8080")
	log.Fatal(http.ListenAndServe(":8080", r))

}

func GetAllPatients(w http.ResponseWriter, r *http.Request) {
	log.Println("GetAllPatients called")

	queryParams := r.URL.Query()
	searchParams := make([]SearchFilter, 0)

	searchParamsMap := map[string]string{
		"family":    "string",
		"birthdate": "date",
		"given":     "string",
	}

	for key, values := range queryParams {
		for _, value := range values {
			typeValue := searchParamsMap[key]
			comparator, paramValue := parseComparator(value, typeValue)
			param := SearchFilter{
				Code:       key,
				Value:      paramValue,
				Type:       typeValue,
				Comparator: comparator,
			}
			searchParams = append(searchParams, param)
		}
	}

	endpoints := util.GetAbsolutePath("config/endpoints2.json")

	file, err := os.Open(endpoints)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	var endpoint Endpoints
	if err := json.NewDecoder(file).Decode(&endpoint); err != nil {
		log.Fatal(err)
	}

	for _, ep := range endpoint.Endpoint {
		for _, sp := range ep.SearchParameter {
			for _, param := range searchParams {
				if sp.Code == param.Code && sp.Value == param.Value {
					// Perform the desired action for matching search parameters
					// You can access the SQL file using ep.SQLFile
					fmt.Println("Matching search parameter found for SQL file:", ep.SQLFile)
				}
			}
		}
	}
	male := fhir.AdministrativeGenderMale

	// Construct a number of FHIR patients with multiple names
	patients := []fhir.Patient{
		{Meta: &fhir.Meta{Id: util.StringPtr("id1")},
			Name: []fhir.HumanName{
				{
					Family: util.StringPtr("Hetty"),
					Given:  []string{"Robert", "Jane"},
				},
			},
			BirthDate: util.StringPtr("1990-01-01"),
			Gender:    &male,
		},
		{
			Name: []fhir.HumanName{
				{
					Family: util.StringPtr("Smith"),
					Given:  []string{"Henk"},
				},
				{
					Family: util.StringPtr("Davis"),
					Given:  []string{"Emily", "Tommy"},
				},
			},
			BirthDate: util.StringPtr("1985-05-10"),
			Gender:    &male,
		},
	}

	// Filter patients based on search parameters
	filteredPatients := make([]fhir.Patient, 0)
	for i, patient := range patients {
		if matchesFilters(patient, searchParams) {
			log.Println("Patient", i, "matches filters")
			filteredPatients = append(filteredPatients, patient)
		}
	}

	filteredPatientsJSON, err := json.Marshal(filteredPatients)
	if err != nil {
		log.Fatal(err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(filteredPatientsJSON)

}

func matchesFilters(patient fhir.Patient, filters []SearchFilter) bool {
	return checkFields(reflect.ValueOf(&patient).Elem(), filters, "Patient")
}
func checkFields(v reflect.Value, filters []SearchFilter, parentField string) bool {
	if v.Kind() != reflect.Struct {
		log.Println("Warning: Value is not a struct")
		return false
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldName := v.Type().Field(i).Name
		fullFieldName := parentField + "." + fieldName

		log.Printf("Checking field: %s with type: %s and kind: %v", fullFieldName, field.Type().String(), field.Kind())

		if matchFound := processField(field, filters, fullFieldName); matchFound {
			return true
		}
	}

	log.Println("No match found in any field")
	return false
}

func processField(field reflect.Value, filters []SearchFilter, fieldName string) bool {
	switch field.Kind() {
	case reflect.Slice:
		return processSlice(field, filters, fieldName)
	case reflect.Struct:
		return checkFields(field, filters, fieldName)
	case reflect.Ptr:
		if !field.IsNil() {
			return processField(field.Elem(), filters, fieldName)
		}
	default:
		return compareBasicType(field, filters)
	}
	return false
}

func processSlice(field reflect.Value, filters []SearchFilter, fieldName string) bool {
	for i := 0; i < field.Len(); i++ {
		element := field.Index(i)
		log.Printf("Checking element %d of slice %s", i, fieldName)
		if matchFound := processField(element, filters, fieldName); matchFound {
			return true
		}
	}
	return false
}

func compareBasicType(field reflect.Value, filters []SearchFilter) bool {
	for _, filter := range filters {
		if compare(field, filter) {
			log.Printf("Match found for field value: %v", field.Interface())
			return true
		}
	}
	return false
}

func compare(field reflect.Value, filter SearchFilter) bool {
	switch field.Kind() {
	case reflect.String:
		return compareString(field.String(), filter)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return compareInt(field.Int(), filter)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return compareUint(field.Uint(), filter)
	case reflect.Float32, reflect.Float64:
		return compareFloat(field.Float(), filter)
	case reflect.Bool:
		return compareBool(field.Bool(), filter)
	}

	// Handle time.Time separately as it's not a reflect.Kind
	if field.Type() == reflect.TypeOf(time.Time{}) {
		return compareTime(field.Interface().(time.Time), filter)
	}

	log.Printf("Unsupported type for comparison: %v", field.Kind())
	return false
}

func compareString(value string, filter SearchFilter) bool {
	switch filter.Comparator {
	case "eq":
		return strings.EqualFold(value, filter.Value)
	case "contains":
		return strings.Contains(strings.ToLower(value), strings.ToLower(filter.Value))
	default:
		return strings.EqualFold(value, filter.Value)
	}
}

func compareInt(value int64, filter SearchFilter) bool {
	filterValue, err := strconv.ParseInt(filter.Value, 10, 64)
	if err != nil {
		log.Printf("Error parsing int filter value: %v", err)
		return false
	}

	switch filter.Comparator {
	case "eq":
		return value == filterValue
	case "gt":
		return value > filterValue
	case "lt":
		return value < filterValue
		// Add more integer comparison operators as needed
	}
	return false
}

func compareUint(value uint64, filter SearchFilter) bool {
	filterValue, err := strconv.ParseUint(filter.Value, 10, 64)
	if err != nil {
		log.Printf("Error parsing uint filter value: %v", err)
		return false
	}

	switch filter.Comparator {
	case "eq":
		return value == filterValue
	case "gt":
		return value > filterValue
	case "lt":
		return value < filterValue
		// Add more unsigned integer comparison operators as needed
	}
	return false
}

func compareFloat(value float64, filter SearchFilter) bool {
	filterValue, err := strconv.ParseFloat(filter.Value, 64)
	if err != nil {
		log.Printf("Error parsing float filter value: %v", err)
		return false
	}

	switch filter.Comparator {
	case "eq":
		return value == filterValue
	case "gt":
		return value > filterValue
	case "lt":
		return value < filterValue
		// Add more float comparison operators as needed
	}
	return false
}

func compareBool(value bool, filter SearchFilter) bool {
	filterValue, err := strconv.ParseBool(filter.Value)
	if err != nil {
		log.Printf("Error parsing bool filter value: %v", err)
		return false
	}

	return value == filterValue
}

func compareTime(value time.Time, filter SearchFilter) bool {
	filterValue, err := time.Parse(time.RFC3339, filter.Value)
	if err != nil {
		log.Printf("Error parsing time filter value: %v", err)
		return false
	}

	switch filter.Comparator {
	case "eq":
		return value.Equal(filterValue)
	case "gt":
		return value.After(filterValue)
	case "lt":
		return value.Before(filterValue)
		// Add more time comparison operators as needed
	}
	return false
}

func parseDateComparator(input string) (string, string) {
	comparators := []string{"eq", "ne", "gt", "lt", "ge", "le"}
	for _, comparator := range comparators {
		if strings.HasPrefix(input, comparator) {
			return comparator, strings.TrimPrefix(input, comparator)
		}
	}

	return "", input
}

func parseComparator(input string, valueType string) (comparator string, paramValue string) {
	switch valueType {
	case "string":
		return "", input
	case "date":
		comparator, value := parseDateComparator(input)
		return comparator, value
	case "integer":
		// Convert input to integer and return
		// Example: intValue, err := strconv.Atoi(input)
	default:
		return "", input
	}

	return "", input
}

func compareSlice(value interface{}, filter SearchFilter) bool {
	sliceValue := value.([]string)
	filterStrValue := filter.Value
	for _, strValue := range sliceValue {
		switch filter.Comparator {
		case "eq", "":
			if strValue == filterStrValue {
				log.Println("String value matched:", strValue)
				return true
			}
		case "ne":
			if strValue != filterStrValue {
				log.Println("String value not matched:", strValue)
				return true
			}
		default:
			log.Println("Invalid comparator:", filter.Comparator)
			return false
		}
	}

	log.Println("No matching string value found in slice", filterStrValue)
	return false
}
