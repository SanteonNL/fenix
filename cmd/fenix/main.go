package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/SanteonNL/fenix/models/fhir/r4"
	"github.com/SanteonNL/fenix/models/sim"
	"github.com/gorilla/mux"

	_ "github.com/jinzhu/gorm/dialects/postgres"
)

func main() {

	// configPath := util.GetAbsolutePath("config/connections.json")

	// file, err := os.Open(configPath)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer file.Close()

	// var config Config
	// if err := json.NewDecoder(file).Decode(&config); err != nil {
	// 	log.Fatal(err)
	// }

	// app := &Application{
	// 	Services: []Service{},
	// }

	// for _, serviceConfig := range config.Services {
	// 	service, err := NewService(serviceConfig)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	app.Services = append(app.Services, service)
	// }

	r := mux.NewRouter()
	// r.HandleFunc("/patient/{id}", app.GetPatient).Methods("GET")
	// r.HandleFunc("/patients/{id}", app.GetAllPatients).Methods("GET")
	r.HandleFunc("/patients2", GetAllPatients2).Methods("GET")
	log.Fatal(http.ListenAndServe(":8080", r))

}

func (app *Application) GetPatient(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	for _, service := range app.Services {
		patient, err := service.GetPatient(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		jsonBytes, err := json.Marshal(patient)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write(jsonBytes)
		return
	}

	http.Error(w, "Patient not found", http.StatusNotFound)
}

func (app *Application) GetAllPatients(w http.ResponseWriter, r *http.Request) {
	//w.Header().Set("Content-Type", "application/fhir+ndjson")

	var allPatients []*fhir.Patient
	for _, service := range app.Services {
		patients, err := service.GetAllPatients()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		allPatients = append(allPatients, patients...)
	}

	jsonBytes, err := json.Marshal(allPatients)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}

func mapRecordToSIMPatient(record []string) (*sim.Patient, error) {
	if len(record) < 2 {
		return nil, fmt.Errorf("invalid record format")
	}

	patient := &sim.Patient{
		Identificatienummer: &record[0],
		Geboortedatum:       parseTime(&record[1]),
		// Continue mapping the rest of the fields...
	}

	return patient, nil
}

func TranslateSIMPatientToFHIR(patient *sim.Patient) (*fhir.Patient, error) {
	fhirPatient := &fhir.Patient{
		Id:        patient.Identificatienummer,
		BirthDate: toString(patient.Geboortedatum),
	}

	return fhirPatient, nil
}

func toString(time *time.Time) *string {
	str := time.Format("2006-01-02")
	return &str
}

func TranslateFHIRPatientToSIM(patient *fhir.Patient) (*sim.Patient, error) {
	simPatient := &sim.Patient{
		Identificatienummer: patient.Id,
		Geboortedatum:       parseTime(patient.BirthDate),
		// Continue for the rest of the fields...
	}

	return simPatient, nil
}

func parseTime(s *string) *time.Time {
	if s == nil {
		return nil
	}
	t, _ := time.Parse("2006-01-02", *s)
	return &t
}

func GetAllPatients2(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello, World!"))
}
