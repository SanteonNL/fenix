package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/SanteonNL/fenix/cmd/fenix/api"
	"github.com/SanteonNL/fenix/cmd/fenix/datasource"
	"github.com/SanteonNL/fenix/cmd/fenix/fhir/conceptmap"
	"github.com/SanteonNL/fenix/cmd/fenix/fhir/fhirpathinfo"
	"github.com/SanteonNL/fenix/cmd/fenix/fhir/searchparameter"
	"github.com/SanteonNL/fenix/cmd/fenix/fhir/structuredefinition"
	"github.com/SanteonNL/fenix/cmd/fenix/fhir/valueset"
	"github.com/SanteonNL/fenix/cmd/fenix/output"
	"github.com/SanteonNL/fenix/cmd/fenix/processor"
	"github.com/SanteonNL/fenix/cmd/fenix/types"
	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

func main() {
	startTime := time.Now()

	log := zerolog.New(os.Stdout).With().Timestamp().Logger()

	outputMgr, err := output.NewOutputManager("output/temp", log)

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create output manager")
	}

	log = outputMgr.GetLogger()

	log.Debug().Msg("Starting fenix")

	// Th	// Initialize database connection
	db, err := sqlx.Connect("postgres", "postgres://postgres:mysecretpassword@localhost:5432/tsl_employee?sslmode=disable")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}

	// Define paths
	baseDir := "."                                                // Current directory
	inputDir := filepath.Join(baseDir, "config/conceptmaps/flat") // ./csv directory for input files
	repoDir := filepath.Join(baseDir, "config/conceptmaps")       // ./fhir directory for repository

	// Initialize repository
	repository := conceptmap.NewConceptMapRepository(repoDir, log)

	// Load existing concept maps
	if err := repository.LoadConceptMaps(); err != nil {
		log.Error().Err(err).Msg("Failed to load existing concept maps")
		os.Exit(1)
	}

	// Initialize services and converter
	conceptMapService := conceptmap.NewConceptMapService(repository, log)
	conceptMapConverter := conceptmap.NewConceptMapConverter(log, conceptMapService)

	// Process the input directory
	log.Info().
		Str("input_dir", inputDir).
		Str("repo_dir", repoDir).
		Msg("Starting conversion of CSV files")

	// Set usePrefix to true to add 'conceptmap_converted_' prefix to ValueSet URIs
	if err := conceptMapConverter.ConvertFolderToFHIR(inputDir, repository, true); err != nil {
		log.Error().Err(err).Msg("Conversion process failed")
		os.Exit(1)
	}

	log.Info().Msg("Successfully completed conversion process")

	dataSourceService := datasource.NewDataSourceService(db, log)
	// Load queries
	//err = dataSourceService.LoadQueryFile("queries/hix/flat/Observation_hix_metingen_metingen.sql")
	err = dataSourceService.LoadQueryFile("queries/hix/flat/patient_1.sql")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load query file")
	}

	// // Read resources
	// results, err := dataSourceService.ReadResources("Patient", "12345")
	// if err != nil {
	// 	log.Printf("Error: %v", err)
	// }

	// // Print results
	// for _, result := range results {
	// 	for path, rowData := range result {
	// 		fmt.Printf("Resource Path: %s, Data: %v\n", path, rowData)
	// 	}
	// }

	// Example: Find ConceptMaps for a specific ValueSet
	valueSetURL := "https://decor.nictiz.nl/fhir/4.0/sansa-/ValueSet/2.16.840.1.113883.2.4.3.11.60.909.11.2--20241203090354"

	conceptMapService.GetConceptMapsByValuesetURL(valueSetURL)

	conceptMaps, err := conceptMapService.GetConceptMapsByValuesetURL(valueSetURL)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get ConceptMaps")
	} else {
		for _, conceptMap := range conceptMaps {
			log.Info().
				Str("name", conceptMap).
				Msg("Found ConceptMap")
		}

	}

	// Create the config
	config := valueset.Config{
		LocalPath:     "valuesets",      // Directory to store ValueSets
		DefaultMaxAge: 24 * time.Hour,   // Cache for 24 hours by default
		HTTPTimeout:   30 * time.Second, // Timeout for remote requests
	}

	// Create the ValueSet service
	valuesetService, err := valueset.NewValueSetService(config, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create ValueSet service")
	}
	// Create a context
	ctx := context.Background()

	// Try getting a remote ValueSet
	remoteValueSet, err := valuesetService.GetValueSet(ctx, "https://decor.nictiz.nl/fhir/4.0/sansa-/ValueSet/2.16.840.1.113883.2.4.3.11.60.909.11.2--20241203090354")
	if err != nil {
		log.Error().Err(err).Msg("Failed to get remote ValueSet")
	} else {
		log.Info().
			Str("id", *remoteValueSet.Id).
			Str("url", *remoteValueSet.Url).
			Msg("Successfully loaded remote ValueSet")
	}

	// Example: Validate a code against a ValueSet
	coding := &fhir.Coding{
		System: ptr("http://snomed.info/sct"),
		Code:   ptr("227620asd02"),
	}

	result, err := valuesetService.ValidateCode(ctx, "https://decor.nictiz.nl/fhir/4.0/sansa-/ValueSet/2.16.840.1.113883.2.4.3.11.60.909.11.2--20241203090354", coding)
	if err != nil {
		log.Error().Err(err).Msg("Failed to validate code")
	} else {
		if result.Valid {
			log.Info().
				Str("matchedIn", result.MatchedIn).
				Msg("Code is valid")
		} else {
			log.Info().
				Str("error", result.ErrorMessage).
				Msg("Code is not valid")
		}
	}

	// Initialize repository for StructureDefinitions
	structureDefRepo := structuredefinition.NewStructureDefinitionRepository(log)

	// Load existing StructureDefinitions
	if err := structureDefRepo.LoadStructureDefinitions("profiles\\sim"); err != nil {
		log.Error().Err(err).Msg("Failed to load existing StructureDefinitions")
		os.Exit(1)
	}

	// Initialize StructureDefinition service with the repository
	structureDefService := structuredefinition.NewStructureDefinitionService(structureDefRepo, log)

	// Initialize repository for SearchParameters
	searchParamRepo := searchparameter.NewSearchParameterRepository(log)
	searchParamRepo.LoadSearchParametersFromFile("searchParameter\\search-parameter.json")

	// Initialize SearchParameter service with the repository
	searchParamService := searchparameter.NewSearchParameterService(searchParamRepo, log)
	searchParamService.BuildSearchParameterIndex()

	searchParamService.DebugResourceSearchParameters("Patient")

	genderSearchType, err := searchParamService.GetSearchTypeByPathAndCode("Patient.gender", "gender")
	if err != nil {
		log.Error().Err(err).Msg("Failed to get SearchParameter")
	} else {
		fmt.Printf("SearchParameter: %+v\n", genderSearchType)
	}

	outputMgr.WriteToJSON(genderSearchType, "searchType")

	pathInfoService := fhirpathinfo.NewPathInfoService(structureDefService, searchParamService, conceptMapService, log)
	structureDefService.BuildStructureDefinitionIndex()

	processorConfig := processor.ProcessorConfig{
		Log:           log,
		PathInfoSvc:   pathInfoService,
		StructDefSvc:  structureDefService,
		ValueSetSvc:   valuesetService,
		ConceptMapSvc: conceptMapService,
		OutputManager: outputMgr,
	}

	processorService, err := processor.NewProcessorService(processorConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create ProcessorService")
	}

	searchType, err := pathInfoService.GetSearchTypeByPathAndCode("Patient.gender", "gender")
	if err != nil {
		log.Error().Err(err).Msg("Failed to get SearchType")
	} else {
		log.Info().Msg("Successfully retrieved SearchType")
		fmt.Printf("SearchType: %+v\n", searchType)
	}
	// Process resources
	filter := types.Filter{
		Code:  "identifier",
		Value: "1s",
	}

	resources, err := processorService.ProcessResources(ctx, dataSourceService, "Patient", "12", []*types.Filter{&filter})
	if err != nil {
		log.Error().Err(err).Msg("Failed to process resources")

	}

	for _, resource := range resources {
		if err := outputMgr.WriteToJSON(resource, "resource"); err != nil {
			log.Error().Err(err).Msg("Failed to write resource to file")
		}
	}

	// Create and setup router
	router := api.NewFHIRRouter(searchParamService, processorService, dataSourceService, log)
	handler := router.SetupRoutes()

	// Start server
	port := ":8080"
	log.Info().Msgf("Starting FHIR server on port %s", port)
	if err := http.ListenAndServe(port, handler); err != nil {
		log.Fatal().Err(err).Msg("Server failed to start")
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	log.Debug().Msgf("Execution time: %s", duration)
}
