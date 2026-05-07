// processor.go
package processor

import (
	"context"
	"fmt"
	"reflect"

	"github.com/SanteonNL/fenix/cmd/fenix/datasource"
	"github.com/SanteonNL/fenix/cmd/fenix/fhir/conceptmap"
	"github.com/SanteonNL/fenix/cmd/fenix/fhir/fhirpathinfo"
	"github.com/SanteonNL/fenix/cmd/fenix/fhir/structuredefinition"
	"github.com/SanteonNL/fenix/cmd/fenix/fhir/valueset"
	"github.com/SanteonNL/fenix/cmd/fenix/output"
	"github.com/SanteonNL/fenix/cmd/fenix/types"
	"github.com/rs/zerolog"
)

type ProcessorService struct {
	log            zerolog.Logger
	pathInfoSvc    *fhirpathinfo.PathInfoService
	structDefSvc   *structuredefinition.StructureDefinitionService
	valueSetSvc    *valueset.ValueSetService
	conceptMapSvc  *conceptmap.ConceptMapService
	outputManager  *output.OutputManager
	processedPaths map[string]bool // Changed from sync.Map for simpler usage
	resourceType   string
	result         datasource.ResourceResult
}

// ProcessorConfig holds all the configuration needed to create a new processor
type ProcessorConfig struct {
	Log           zerolog.Logger
	PathInfoSvc   *fhirpathinfo.PathInfoService
	StructDefSvc  *structuredefinition.StructureDefinitionService
	ValueSetSvc   *valueset.ValueSetService
	ConceptMapSvc *conceptmap.ConceptMapService
	OutputManager *output.OutputManager
}

// NewProcessorService creates a new processor service with all required dependencies
func NewProcessorService(config ProcessorConfig) (*ProcessorService, error) {
	if config.PathInfoSvc == nil {
		return nil, fmt.Errorf("pathInfoSvc is required")
	}
	if config.ValueSetSvc == nil {
		return nil, fmt.Errorf("valueSetSvc is required")
	}
	if config.ConceptMapSvc == nil {
		return nil, fmt.Errorf("conceptMapSvc is required")
	}
	if config.OutputManager == nil {
		return nil, fmt.Errorf("outputManager is required")
	}

	return &ProcessorService{
		log:            config.Log,
		pathInfoSvc:    config.PathInfoSvc,
		structDefSvc:   config.StructDefSvc,
		valueSetSvc:    config.ValueSetSvc,
		conceptMapSvc:  config.ConceptMapSvc,
		outputManager:  config.OutputManager,
		processedPaths: make(map[string]bool),
	}, nil
}

// ProcessResources processes resources with filtering
func (p *ProcessorService) ProcessResources(ctx context.Context, ds *datasource.DataSourceService, resourceType string, patientID string, filter []*types.Filter) ([]interface{}, error) {
	results, err := ds.ReadResources(resourceType, patientID)
	if err != nil {
		return nil, fmt.Errorf("failed to read resources: %w", err)
	}

	err = p.outputManager.WriteToJSON(results, "temp_result")
	if err != nil {
		return nil, fmt.Errorf("failed to write resources to JSON: %w", err)
	}

	var processedResources []interface{}
	for _, result := range results {
		// Reset processor state for new resource
		p.processedPaths = make(map[string]bool)
		p.result = result
		p.resourceType = resourceType // Set from parameter directly

		processed, err := p.ProcessSingleResource(result, filter)
		if err != nil {
			p.log.Error().Err(err).Msg("Error processing resource")
			continue
		}
		if processed != nil {
			processedResources = append(processedResources, processed)
		}
	}

	return processedResources, nil
}

// ProcessPreloadedResources processes already-fetched resource results without re-querying the database.
func (p *ProcessorService) ProcessPreloadedResources(ctx context.Context, results []datasource.ResourceResult, resourceType string, filter []*types.Filter) ([]interface{}, error) {
	err := p.outputManager.WriteToJSON(results, "temp_result")
	if err != nil {
		return nil, fmt.Errorf("failed to write resources to JSON: %w", err)
	}

	var processedResources []interface{}
	for _, result := range results {
		p.processedPaths = make(map[string]bool)
		p.result = result
		p.resourceType = resourceType

		processed, err := p.ProcessSingleResource(result, filter)
		if err != nil {
			p.log.Error().Err(err).Msg("Error processing resource")
			continue
		}
		if processed != nil {
			processedResources = append(processedResources, processed)
		}
	}

	return processedResources, nil
}

// ProcessSingleResource processes a single resource
func (p *ProcessorService) ProcessSingleResource(result datasource.ResourceResult, filter []*types.Filter) (interface{}, error) {
	// Create resource
	resource, err := p.createResource()
	if err != nil {
		return nil, fmt.Errorf("error creating resource: %w", err)
	}

	// Populate and filter resource
	passed, err := p.populateResourceStruct(reflect.ValueOf(resource).Elem(), filter)
	if err != nil {
		return nil, fmt.Errorf("error populating resource: %w", err)
	}

	err = p.outputManager.WriteToJSON(resource, "result")
	if err != nil {
		return nil, fmt.Errorf("failed to write resources to JSON: %w", err)
	}

	if !passed {
		return nil, nil
	}

	return resource, nil
}

// createResource creates a new instance of the appropriate resource type
func (p *ProcessorService) createResource() (interface{}, error) {
	factory, exists := ResourceFactoryMap[p.resourceType]
	if !exists {
		return nil, fmt.Errorf("unsupported resource type: %s", p.resourceType)
	}
	return factory(), nil
}

// GetConceptMapService returns the ConceptMapService
func (p *ProcessorService) GetConceptMapService() *conceptmap.ConceptMapService {
	return p.conceptMapSvc
}
