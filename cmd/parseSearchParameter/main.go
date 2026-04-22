package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

type Filter struct {
	Value    string
	Modifier string
	Type     string
}

type SearchParameterInfo struct {
	SearchParameter fhir.SearchParameter
	Filter          Filter
}

type Bundle struct {
	ResourceType string `json:"resourceType"`
	Id           string `json:"id"`
	Meta         struct {
		LastUpdated string `json:"lastUpdated"`
	} `json:"meta"`
	Type  string `json:"type"`
	Entry []struct {
		FullUrl  string          `json:"fullUrl"`
		Resource json.RawMessage `json:"resource"`
	} `json:"entry"`
}

func parseSearchParameters(jsonData []byte) (map[string]map[string]map[string]SearchParameterInfo, error) {
	var bundle Bundle

	err := json.Unmarshal(jsonData, &bundle)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON: %v", err)
	}

	if bundle.ResourceType != "Bundle" || bundle.Type != "collection" {
		return nil, fmt.Errorf("invalid bundle: expected ResourceType 'Bundle' and Type 'collection'")
	}

	result := make(map[string]map[string]map[string]SearchParameterInfo)

	for _, entry := range bundle.Entry {
		var sp fhir.SearchParameter

		err := json.Unmarshal(entry.Resource, &sp)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling SearchParameter: %v", err)
		}

		for _, base := range sp.Base {
			baseString := base.String()
			if _, ok := result[baseString]; !ok {
				result[baseString] = make(map[string]map[string]SearchParameterInfo)
			}

			expression := *sp.Expression
			expressionParts := strings.Split(expression, "|")
			for _, expr := range expressionParts {
				expr = strings.TrimSpace(expr)
				if strings.HasPrefix(expr, baseString+".") || expr == baseString {
					if _, ok := result[baseString][expr]; !ok {
						result[baseString][expr] = make(map[string]SearchParameterInfo)
					}
					result[baseString][expr][sp.Code] = SearchParameterInfo{
						SearchParameter: sp,
						Filter:          Filter{},
					}
				}
			}
		}
	}

	return result, nil
}
func main() {
	// Example usage
	jsonData := []byte(`
{
  "resourceType": "Bundle",
  "id": "searchParams",
  "meta": {
    "lastUpdated": "2023-03-26T15:21:02.749+11:00"
  },
  "type": "collection",
  "entry": [
    {
      "fullUrl": "http://hl7.org/fhir/SearchParameter/Patient-identifier",
      "resource": {
        "resourceType": "SearchParameter",
        "id": "Patient-identifier",
        "extension": [
          {
            "url": "http://hl7.org/fhir/StructureDefinition/structuredefinition-standards-status",
            "valueCode": "normative"
          }
        ],
        "url": "http://hl7.org/fhir/SearchParameter/Patient-identifier",
        "version": "5.0.0",
        "name": "identifier",
        "status": "active",
        "experimental": false,
        "date": "2023-03-26T15:21:02+11:00",
        "publisher": "Health Level Seven International (Patient Administration)",
        "contact": [
          {
            "telecom": [
              {
                "system": "url",
                "value": "http://hl7.org/fhir"
              }
            ]
          },
          {
            "telecom": [
              {
                "system": "url",
                "value": "http://www.hl7.org/Special/committees/pafm/index.cfm"
              }
            ]
          }
        ],
        "description": "A patient identifier",
        "jurisdiction": [
          {
            "coding": [
              {
                "system": "http://unstats.un.org/unsd/methods/m49/m49.htm",
                "code": "001",
                "display": "World"
              }
            ]
          }
        ],
        "code": "identifier",
        "base": ["Patient"],
        "type": "token",
        "expression": "Patient.identifier",
        "processingMode": "normal"
      }
    },
    {
      "fullUrl": "http://hl7.org/fhir/SearchParameter/individual-gender",
      "resource": {
        "resourceType": "SearchParameter",
        "id": "individual-gender",
        "extension": [
          {
            "url": "http://hl7.org/fhir/StructureDefinition/structuredefinition-standards-status",
            "valueCode": "normative"
          }
        ],
        "url": "http://hl7.org/fhir/SearchParameter/individual-gender",
        "version": "5.0.0",
        "name": "gender",
        "status": "active",
        "experimental": false,
        "date": "2023-03-26T15:21:02+11:00",
        "publisher": "Health Level Seven International (Patient Administration)",
        "contact": [
          {
            "telecom": [
              {
                "system": "url",
                "value": "http://hl7.org/fhir"
              }
            ]
          },
          {
            "telecom": [
              {
                "system": "url",
                "value": "http://www.hl7.org/Special/committees/pafm/index.cfm"
              }
            ]
          }
        ],
        "description": "Multiple Resources: \r\n\r\n* [Patient](patient.html): Gender of the patient\r\n* [Person](person.html): The gender of the person\r\n* [Practitioner](practitioner.html): Gender of the practitioner\r\n* [RelatedPerson](relatedperson.html): Gender of the related person\r\n",
        "jurisdiction": [
          {
            "coding": [
              {
                "system": "http://unstats.un.org/unsd/methods/m49/m49.htm",
                "code": "001",
                "display": "World"
              }
            ]
          }
        ],
        "code": "gender",
        "base": ["Patient", "Person", "Practitioner", "RelatedPerson"],
        "_base": [
          null,
          {
            "extension": [
              {
                "url": "http://hl7.org/fhir/StructureDefinition/structuredefinition-standards-status",
                "valueCode": "trial-use"
              }
            ]
          },
          {
            "extension": [
              {
                "url": "http://hl7.org/fhir/StructureDefinition/structuredefinition-standards-status",
                "valueCode": "trial-use"
              }
            ]
          },
          {
            "extension": [
              {
                "url": "http://hl7.org/fhir/StructureDefinition/structuredefinition-standards-status",
                "valueCode": "trial-use"
              }
            ]
          }
        ],
        "type": "token",
        "expression": "Patient.gender | Person.gender | Practitioner.gender | RelatedPerson.gender",
        "processingMode": "normal"
      }
    }
  ]
}
`)

	result, err := parseSearchParameters(jsonData)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	//fmt.Printf("%+v\n", result)

	fmt.Printf("SearchParameterInfo for Patient.identifier: %+v\n", result["Person"]["Person.gender"]["gender"].SearchParameter.Base)
}
