// File: internal/converter/converter.go
package converter

import (
	"github.com/SanteonNL/fenix/cmd/artdecor/types" // Update with your actual module path
	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// ConvertToFHIRConceptMap converts an Art-Decor ConceptMap to a FHIR ConceptMap
func ConvertToFHIRConceptMap(decorMap types.DECORConceptMap) fhir.ConceptMap {

	dateStr := string(*decorMap.EffectiveDate)

	fhirMap := fhir.ConceptMap{
		Name:        &decorMap.DisplayName,
		Identifier:  &fhir.Identifier{Value: decorMap.Ident},
		Status:      getPublicationStatus(decorMap.StatusCode),
		Date:        &dateStr,
		Description: getDescription(decorMap.Desc),
		Purpose:     getPurpose(decorMap.Purpose),
		Copyright:   getCopyright(decorMap.Copyright),
	}

	if decorMap.Url != nil {
		fhirMap.Url = decorMap.Url
	}

	// Handle source and target scopes
	if len(decorMap.SourceScope) > 0 && decorMap.SourceScope[0].CanonicalUri != nil {
		fhirMap.SourceUri = decorMap.SourceScope[0].CanonicalUri
	}
	if len(decorMap.TargetScope) > 0 && decorMap.TargetScope[0].CanonicalUri != nil {
		fhirMap.TargetUri = decorMap.TargetScope[0].CanonicalUri
	}

	// Convert groups
	fhirMap.Group = convertGroups(decorMap.Group)

	return fhirMap
}

// Convert DECORConceptMap groups to FHIR ConceptMap groups
func convertGroups(decorGroups []*types.ConceptMapGroupDefinition) []fhir.ConceptMapGroup {
	var fhirGroups []fhir.ConceptMapGroup

	for _, decorGroup := range decorGroups {
		if decorGroup == nil {
			continue
		}

		fhirGroup := fhir.ConceptMapGroup{
			Element: convertElements(decorGroup.Element),
		}

		// Set source and target from the first entries if they exist
		if len(decorGroup.Source) > 0 && decorGroup.Source[0].CanoncialUri != nil {
			fhirGroup.Source = decorGroup.Source[0].CanoncialUri
		}
		if len(decorGroup.Target) > 0 && decorGroup.Target[0].CanoncialUri != nil {
			fhirGroup.Target = decorGroup.Target[0].CanoncialUri
		}

		fhirGroups = append(fhirGroups, fhirGroup)
	}

	return fhirGroups
}

// Convert ConceptMapElement to FHIR ConceptMapGroupElement
func convertElements(decorElements []types.ConceptMapElement) []fhir.ConceptMapGroupElement {
	var fhirElements []fhir.ConceptMapGroupElement

	for _, decorElement := range decorElements {
		fhirElement := fhir.ConceptMapGroupElement{
			Code:    decorElement.Code,
			Display: decorElement.DisplayName,
			Target:  convertTargets(decorElement.Target),
		}

		fhirElements = append(fhirElements, fhirElement)
	}

	return fhirElements
}

// Convert ConceptMapTarget to FHIR ConceptMapGroupElementTarget
func convertTargets(decorTargets []*types.ConceptMapTarget) []fhir.ConceptMapGroupElementTarget {
	var fhirTargets []fhir.ConceptMapGroupElementTarget

	for _, decorTarget := range decorTargets {
		if decorTarget == nil {
			continue
		}

		fhirTarget := fhir.ConceptMapGroupElementTarget{
			Code:        decorTarget.Code,
			Display:     decorTarget.DisplayName,
			Equivalence: convertEquivalence(decorTarget.Relationship),
		}

		fhirTargets = append(fhirTargets, fhirTarget)
	}

	return fhirTargets
}

// Helper functions for converting specific fields
func getPublicationStatus(statusCode *string) fhir.PublicationStatus {
	if statusCode == nil {
		return fhir.PublicationStatusDraft
	}
	switch *statusCode {
	case "active":
		return fhir.PublicationStatusActive
	case "retired":
		return fhir.PublicationStatusRetired
	default:
		return fhir.PublicationStatusDraft
	}
}

func getDescription(desc []*types.FreeFormMarkupWithLanguage) *string {
	if len(desc) == 0 {
		return nil
	}
	return &desc[0].Text
}

func getPurpose(purpose []*types.FreeFormMarkupWithLanguage) *string {
	if len(purpose) == 0 {
		return nil
	}
	return &purpose[0].Text
}

func getCopyright(copyright []*types.CopyrightText) *string {
	if len(copyright) == 0 {
		return nil
	}
	return &copyright[0].Text
}

func convertEquivalence(relationship *string) fhir.ConceptMapEquivalence {
	if relationship == nil {
		return fhir.ConceptMapEquivalenceEquivalent
	}
	switch *relationship {
	case "equal":
		return fhir.ConceptMapEquivalenceEqual
	case "wider":
		return fhir.ConceptMapEquivalenceWider
	case "narrower":
		return fhir.ConceptMapEquivalenceNarrower
	case "inexact":
		return fhir.ConceptMapEquivalenceInexact
	default:
		return fhir.ConceptMapEquivalenceEquivalent
	}
}
