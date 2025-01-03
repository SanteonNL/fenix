package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/SanteonNL/fenix/models/fhir"
	"github.com/SanteonNL/fenix/util"
	"github.com/joho/godotenv"
)

var (
	encounters   fhir.Bundle
	observations map[string]fhir.Bundle
	outputDir    string
	logFile      *os.File
	logger       *log.Logger
)

func init() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}

	outputDir = os.Getenv("OUTPUT_DIR")
	if outputDir == "" {
		log.Fatal("OUTPUT_DIR not set in .env file")
	}

	err = os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		log.Fatalf("Error creating output directory: %v", err)
	}

	// Create log file
	logFile, err = os.OpenFile(filepath.Join(outputDir, "requests.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening log file: %v", err)
	}
	logger = log.New(logFile, "", log.LstdFlags)

	// Load FHIR resources
	inputDir := os.Getenv("INPUT_DIR")
	if inputDir == "" {
		log.Fatal("INPUT_DIR not set in .env file")
	}

	encounters = loadBundle(util.GetAbsolutePath(inputDir + "/encounters.json"))
	observations = map[string]fhir.Bundle{
		"P001": loadBundle(util.GetAbsolutePath(inputDir + "/observations-p001.json")),
		"P002": loadBundle(util.GetAbsolutePath(inputDir + "/observations-p002.json")),
		"P003": loadBundle(util.GetAbsolutePath(inputDir + "/observations-p003.json")),
	}
}

func saveOutput(handlerName string, data interface{}) (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s_%s.json", handlerName, timestamp)
	filePath := filepath.Join(outputDir, filename)

	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return "", err
	}

	return filePath, nil
}

func logRequest(r *http.Request, outputFile string) {
	logger.Printf("Request: %s %s Output: %s\n", r.Method, r.URL.String(), outputFile)
}

func encounterHandler(w http.ResponseWriter, r *http.Request) {
	locationType := r.URL.Query().Get("location.type")
	status := r.URL.Query().Get("status")

	var result interface{}

	if locationType == "ICU" && status == "in-progress" {
		filteredEntries := []fhir.BundleEntry{}
		for _, entry := range encounters.Entry {
			filteredEntries = append(filteredEntries, entry)
		}

		lenFilteredEntries := len(filteredEntries)
		result = &fhir.Bundle{
			Type:  fhir.BundleTypeSearchset,
			Total: &lenFilteredEntries,
			Entry: filteredEntries,
		}
	} else {
		http.Error(w, "No matching encounters found", http.StatusNotFound)
		return
	}

	outputFile, err := saveOutput("encounter", result)
	if err != nil {
		log.Printf("Error saving output: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	logRequest(r, outputFile)

	json.NewEncoder(w).Encode(result)
}

func observationHandler(w http.ResponseWriter, r *http.Request) {
	patient := r.URL.Query().Get("patient")

	var result interface{}

	if obs, ok := observations[patient]; ok {
		result = obs
	} else {
		http.Error(w, "No observations found for the specified patient", http.StatusNotFound)
		return
	}

	outputFile, err := saveOutput("observation", result)
	if err != nil {
		log.Printf("Error saving output: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	logRequest(r, outputFile)

	json.NewEncoder(w).Encode(result)
}

func main() {
	defer logFile.Close()

	http.HandleFunc("/Encounter", encounterHandler)
	http.HandleFunc("/Observation", observationHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // default port if not specified
	}

	fmt.Printf("Server is running on http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
func loadBundle(filename string) fhir.Bundle {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	defer file.Close()

	byteValue, _ := io.ReadAll(file)
	var bundle fhir.Bundle
	err = json.Unmarshal(byteValue, &bundle)
	if err != nil {
		log.Fatalf("Error unmarshalling bundle: %v", err)
	}

	return bundle
}
