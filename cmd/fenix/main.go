package main

import (
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
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

func main() {
	startTime := time.Now()

	log := zerolog.New(os.Stdout).With().Timestamp().Logger()

	outputMgr, err := output.NewOutputManager("./output/temp", log)

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create output manager")
	}

	log = outputMgr.GetLogger()

	log.Debug().Msg("Starting fenix")

	// Th	// Initialize database connection
	db, err := sqlx.Connect("postgres", "postgres://postgres:postgres@localhost:5432/development?sslmode=disable")
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

	// Initialize repository for StructureDefinitions
	structureDefRepo := structuredefinition.NewStructureDefinitionRepository(log)

	// Load existing StructureDefinitions
	if err := structureDefRepo.LoadStructureDefinitions("profiles/sim"); err != nil {
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

	//searchParamService.DebugResourceSearchParameters("Patient")

	genderSearchType, err := searchParamService.GetSearchTypeByPathAndCode("Patient.gender", "gender")
	if err != nil {
		log.Error().Err(err).Msg("Failed to get SearchParameter")
	} else {
		fmt.Printf("SearchParameter: %+v\n", genderSearchType)
	}

	//outputMgr.WriteToJSON(genderSearchType, "searchType")

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

	// Create and setup router
	router := api.NewFHIRRouter(searchParamService, processorService, dataSourceService, log)
	handler := router.SetupRoutes()

	// Start server
	port := ":8081"
	log.Info().Msgf("Starting FHIR server on port %s", port)
	if err := http.ListenAndServe(port, handler); err != nil {
		log.Fatal().Err(err).Msg("Server failed to start")
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	log.Debug().Msgf("Execution time: %s", duration)
}
