package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

type ResourceLoader struct {
	valueSetCache *ValueSetCache
	conceptMaps   map[string]*fhir.ConceptMap
	baseDir       string
	logger        zerolog.Logger
}

func NewResourceLoader(baseDir string, logger zerolog.Logger) *ResourceLoader {
	return &ResourceLoader{
		valueSetCache: NewValueSetCache(filepath.Join(baseDir, "valuesets"), logger),
		conceptMaps:   make(map[string]*fhir.ConceptMap),
		baseDir:       baseDir,
		logger:        logger,
	}
}

// LoadResources loads ConceptMaps and their referenced ValueSets
func (l *ResourceLoader) LoadResources() error {
	// First load ConceptMaps to identify needed ValueSets
	if err := l.loadConceptMaps(); err != nil {
		return fmt.Errorf("failed to load concept maps: %w", err)
	}

	// ValueSets will be loaded on-demand by the cache
	return nil
}

func (l *ResourceLoader) loadConceptMaps() error {
	conceptMapDir := filepath.Join(l.baseDir, "conceptmaps", "original")
	files, err := os.ReadDir(conceptMapDir)
	if err != nil {
		return fmt.Errorf("failed to read conceptmaps directory: %w", err)
	}

	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".json" {
			filePath := filepath.Join(conceptMapDir, file.Name())

			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read concept map file %s: %w", file.Name(), err)
			}

			var cm fhir.ConceptMap
			if err := json.Unmarshal(data, &cm); err != nil {
				return fmt.Errorf("failed to parse concept map %s: %w", file.Name(), err)
			}

			l.conceptMaps[file.Name()] = &cm
			l.logger.Debug().Str("file", file.Name()).Msg("Loaded ConceptMap")
		}
	}

	return nil
}

func (l *ResourceLoader) FixConceptMaps() error {
	l.logger.Info().Msg("Starting to fix ConceptMaps")
	outputDir := filepath.Join(l.baseDir, "conceptmaps", "fixed")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	for name, cm := range l.conceptMaps {
		l.logger.Info().Str("conceptMap", name).Msg("Processing ConceptMap")
		if cm.SourceUri == nil || cm.TargetUri == nil {
			l.logger.Warn().
				Str("conceptMap", name).
				Msg("ConceptMap missing source or target URI")
			continue
		}

		l.logger.Info().
			Str("conceptMap", name).
			Msg("Rebuilding ConceptMap")
		fixedCM, err := RebuildConceptMap(cm, l.valueSetCache, l.logger)
		if err != nil {
			l.logger.Warn().
				Err(err).
				Str("conceptMap", name).
				Msg("Failed to fix ConceptMap")
			continue
		}

		// Save fixed ConceptMap
		outPath := filepath.Join(outputDir, name)
		data, err := json.MarshalIndent(fixedCM, "", "    ")
		if err != nil {
			l.logger.Error().
				Err(err).
				Str("conceptMap", name).
				Msg("Failed to marshal fixed ConceptMap")
			continue
		}

		if err := os.WriteFile(outPath, data, 0644); err != nil {
			l.logger.Error().
				Err(err).
				Str("conceptMap", name).
				Msg("Failed to write fixed ConceptMap")
			continue
		}

		l.logger.Info().
			Str("conceptMap", name).
			Msg("Fixed and saved ConceptMap")
	}

	l.logger.Info().Msg("Finished fixing ConceptMaps")
	return nil
}

func (l *ResourceLoader) GetValueSet(uri string) (*fhir.ValueSet, error) {
	return l.valueSetCache.GetValueSet(uri)
}

func (l *ResourceLoader) GetConceptMaps() map[string]*fhir.ConceptMap {
	return l.conceptMaps
}

// SystemCodeMap maps system URLs to their codes and displays
type SystemCodeMap struct {
	Codes    map[string]bool
	Displays map[string]string
}

type CodeSystemInfo struct {
	System  string
	Code    string
	Display string
}

type CodeInfo struct {
	System  string
	Code    string
	Display string
}

func RebuildConceptMap(cm *fhir.ConceptMap, vsCache *ValueSetCache, logger zerolog.Logger) (*fhir.ConceptMap, error) {
	newCM := &fhir.ConceptMap{
		Id:           cm.Id,
		Meta:         cm.Meta,
		Language:     cm.Language,
		Identifier:   cm.Identifier,
		Name:         cm.Name,
		Title:        cm.Title,
		Status:       cm.Status,
		Experimental: cm.Experimental,
		Date:         cm.Date,
		Publisher:    cm.Publisher,
		Contact:      cm.Contact,
		Description:  cm.Description,
		UseContext:   cm.UseContext,
		Purpose:      cm.Purpose,
		Copyright:    cm.Copyright,
		SourceUri:    cm.SourceUri,
		TargetUri:    cm.TargetUri,
	}

	var newGroups []fhir.ConceptMapGroup

	// Process each original group
	for _, group := range cm.Group {
		if group.Source == nil || group.Target == nil {
			continue
		}

		logger.Debug().
			Str("sourceValueSet", *group.Source).
			Str("targetValueSet", *group.Target).
			Msg("Processing group")

		// Get ValueSets referenced in group source/target
		sourceVS, err := vsCache.GetValueSet(*group.Source)
		if err != nil {
			return nil, fmt.Errorf("failed to get source ValueSet: %w", err)
		}

		targetVS, err := vsCache.GetValueSet(*group.Target)
		if err != nil {
			return nil, fmt.Errorf("failed to get target ValueSet: %w", err)
		}

		// Create maps to track system groupings
		systemGroupings := make(map[string]map[string][]fhir.ConceptMapGroupElement)

		// Process each element in the group
		for _, elem := range group.Element {
			if elem.Code == nil {
				continue
			}

			// Find source systems for this code
			sourceSystems := findSystemsForCode(sourceVS, *elem.Code)
			logger.Debug().
				Str("code", *elem.Code).
				Interface("systems", sourceSystems).
				Msg("Found source systems for code")

			// Process each target in the element

			for _, target := range elem.Target {
				if target.Code == nil {
					continue
				}

				// Find target systems for this code
				targetSystems := findSystemsForCode(targetVS, *target.Code)
				logger.Debug().
					Str("code", *target.Code).
					Interface("systems", targetSystems).
					Msg("Found target systems for code")

				// If systems were found for both source and target, create the mappings
				for _, sourceInfo := range sourceSystems {
					for _, targetInfo := range targetSystems {
						// Initialize maps if needed
						if _, exists := systemGroupings[sourceInfo.System]; !exists {
							systemGroupings[sourceInfo.System] = make(map[string][]fhir.ConceptMapGroupElement)
						}

						// Create element for this system combination
						newElement := fhir.ConceptMapGroupElement{
							Code:    elem.Code,
							Display: &sourceInfo.Display,
							Target: []fhir.ConceptMapGroupElementTarget{
								{
									Code:        target.Code,
									Display:     &targetInfo.Display,
									Equivalence: target.Equivalence,
									Comment:     target.Comment,
								},
							},
						}

						systemGroupings[sourceInfo.System][targetInfo.System] = append(
							systemGroupings[sourceInfo.System][targetInfo.System],
							newElement,
						)

						logger.Debug().
							Str("sourceSystem", sourceInfo.System).
							Str("targetSystem", targetInfo.System).
							Str("sourceCode", *elem.Code).
							Str("targetCode", *target.Code).
							Msg("Added mapping to system group")
					}
				}
			}
		}

		// Create groups from the collected mappings
		for sourceSystem, targetGroups := range systemGroupings {
			for targetSystem, elements := range targetGroups {
				if len(elements) > 0 {
					newGroups = append(newGroups, fhir.ConceptMapGroup{
						Source:  &sourceSystem,
						Target:  &targetSystem,
						Element: elements,
					})

					logger.Debug().
						Str("sourceSystem", sourceSystem).
						Str("targetSystem", targetSystem).
						Int("elementCount", len(elements)).
						Msg("Created new group")
				}
			}
		}
	}

	newCM.Group = newGroups
	return newCM, nil
}

// findSystemsForCode looks up a code in a ValueSet and returns all systems that contain it
func findSystemsForCode(vs *fhir.ValueSet, code string) []CodeInfo {
	var systems []CodeInfo

	if vs == nil || vs.Compose == nil {
		return systems
	}

	for _, include := range vs.Compose.Include {
		if include.System == nil {
			continue
		}

		system := *include.System

		// Special handling for wildcard
		if code == "*" {
			// For wildcard, add the system with the wildcard code
			systems = append(systems, CodeInfo{
				System:  system,
				Code:    "*",
				Display: "Unknown",
			})
			continue
		}

		// Look for exact code match
		for _, concept := range include.Concept {
			if concept.Code == code {
				info := CodeInfo{
					System:  system,
					Code:    code,
					Display: code, // Default to code if no display
				}
				if concept.Display != nil {
					info.Display = *concept.Display
				}
				systems = append(systems, info)
				break
			}
		}
	}

	return systems
}
