// File: cmd/conceptmaps/main.go
package main

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"path/filepath"

	"github.com/SanteonNL/fenix/cmd/artdecor/client"
	"github.com/SanteonNL/fenix/cmd/artdecor/converter"
	"github.com/SanteonNL/fenix/cmd/artdecor/internal/utils"
	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Load(".env"); err != nil {
		log.Default().Fatal("Error loading .env file")
	}
}

func main() {
	c := client.NewArtDecorApiClient()
	token, err := c.Token()
	if err != nil {
		log.Default().Fatal(err)
	}
	c.SetToken(token)

	cms, err := c.ReadConceptMap(map[string]string{
		"prefix": os.Getenv("ART_PROJECT"),
		"sort":   "displayName",
		"search": os.Getenv("ORGANIZATION"),
	})
	if err != nil {
		log.Default().Fatal(err)
	}

	// Filter concept maps
	cms = utils.FilterConceptMaps(cms, os.Getenv("ORGANIZATION")+"_"+os.Getenv("SOURCE")+"_")

	var ARTDECOR_PATH = "../../config/conceptmaps/artdecor"
	var FHIR_PATH = "../../config/conceptmaps/fhir"
	var FORMAT = "json"

	// Create directories if they don't exist
	for _, dir := range []string{ARTDECOR_PATH, FHIR_PATH} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Default().Fatal(err)
		}
	}

	for _, cm := range *cms {
		// Download and save ART-DECOR concept map
		downloadURI, err := url.JoinPath(os.Getenv("ART_DOWNLOAD_URL"), *cm.Ident, "ConceptMap", cm.Id.String())
		if err != nil {
			log.Default().Fatal(err)
		}
		downloadURI += "?_format=" + FORMAT

		artDecorFile, err := filepath.Abs(filepath.Join(ARTDECOR_PATH, cm.DisplayName+"."+FORMAT))
		if err != nil {
			log.Default().Fatal(err)
		}
		log.Default().Printf("Downloading ART-DECOR map: %s --> %s", downloadURI, artDecorFile)

		if err := utils.DownloadFile(artDecorFile, downloadURI); err != nil {
			log.Default().Printf("Error downloading file: %v", err)
			continue
		}

		// Convert to FHIR and save
		fhirCM := converter.ConvertToFHIRConceptMap(cm)
		fhirFile, err := filepath.Abs(filepath.Join(FHIR_PATH, cm.DisplayName+"."+FORMAT))
		if err != nil {
			log.Default().Fatal(err)
		}

		fhirJSON, err := json.MarshalIndent(fhirCM, "", "  ")
		if err != nil {
			log.Default().Printf("Error marshaling FHIR concept map %s: %v", cm.DisplayName, err)
			continue
		}

		if err := os.WriteFile(fhirFile, fhirJSON, 0644); err != nil {
			log.Default().Printf("Error writing FHIR concept map %s: %v", fhirFile, err)
			continue
		}
		log.Default().Printf("Saved FHIR concept map: %s", fhirFile)
	}
}
