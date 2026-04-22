package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SanteonNL/fenix/internal/util"
	"github.com/jmoiron/sqlx"
)

func main() {
	// Specify the folder path where the queries are located
	folderPath := util.GetAbsolutePath("queries/hix")
	outputPath := util.GetAbsolutePath("cmd/sqlGenerator/output")

	// Create outputPath if it doesn't exist
	err := os.MkdirAll(outputPath, os.ModePerm)
	if err != nil {
		fmt.Println("Error creating output folder:", err)
		return
	}

	// Get a list of all files in the folder
	files, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Println("Error reading folder:", err)
		return
	}

	// Iterate over each file in the folder
	for _, file := range files {
		// Skip directories
		if file.IsDir() {
			continue
		}
		fileName := file.Name()

		if fileName != "patient.sql" {
			continue
		}

		fmt.Println("File name:", fileName)
		// Read the contents of the file
		filePath := filepath.Join(folderPath, fileName)
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Println("Error reading file:", err)
			continue
		}

		// Convert queryContent from []byte to string
		queryString := string(content)
		// Regular expression to match "-- *.sql"
		re := regexp.MustCompile(`-- (\w+\.sql)`)

		// Find all matches in the content
		matches := re.FindAllStringSubmatch(queryString, -1)
		for _, match := range matches {
			fmt.Println("SQL file name:", match[1])

			// Read the contents of the matched file
			matchedFilePath := filepath.Join(folderPath, match[1])
			matchedContent, err := os.ReadFile(matchedFilePath)
			if err != nil {
				fmt.Println("Error reading matched file:", err)
				continue
			}

			// Convert matchedContent from []byte to string
			matchedQueryString := string(matchedContent)

			// Replace the regular expression match with the matched content
			queryString = strings.ReplaceAll(queryString, match[0], matchedQueryString)
		}

		// Use sqlx to add named parameters
		namedParams := map[string]interface{}{
			"birthdate":          "20120101",
			"birthdate.operator": "<=",
			"given":              "tommy",
		}

		// Prepare the query with named parameters
		query, args, err := sqlx.BindNamed(sqlx.DOLLAR, queryString, namedParams)
		if err != nil {
			fmt.Println("Error preparing query with named parameters:", err)
			continue
		}

		// Manually replace the placeholders with the actual values
		for i, arg := range args {
			placeholder := fmt.Sprintf("$%d", i+1)
			value := fmt.Sprintf("'%v'", arg)
			query = strings.Replace(query, placeholder, value, 1)
		}

		fmt.Println(query)

		// Write the formatted query to a new file
		newFilePath := filepath.Join(outputPath, fileName)
		err = os.WriteFile(newFilePath, []byte(query), 0644)
		if err != nil {
			fmt.Println("Error writing formatted query:", err)
			continue
		}
		fmt.Println("Formatted query written to:", newFilePath)

	}
}
