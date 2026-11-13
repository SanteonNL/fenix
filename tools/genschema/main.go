// Command genschema generates fenix-config.schema.json, the JSON Schema for Fenix's
// YAML config file, from the config structs in github.com/SanteonNL/fenix/config.
//
// Editors use the schema to show a description on hover, autocomplete field names and
// underline invalid fields (VS Code: Red Hat YAML extension).
//
// This program lives in the separate tools module on purpose: the schema library is a
// development tool and must never become a dependency of the Fenix program itself.
//
// The doc comments of the config structs become the schema descriptions, so even a change
// to only a description needs a regenerated schema.
//
// Usage, from the repository root (after changing a config struct; commit the result):
//
//	make schema
//	go generate ./config/...
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/SanteonNL/fenix/config"
	"github.com/invopop/jsonschema"
)

const modulePath = "github.com/SanteonNL/fenix"

// sourceTypeFields lists, per source `type`, which fields that type reads and which it requires.
type sourceTypeFields struct {
	allowed  []string
	required []string
}

// fieldsBySourceType mirrors the constructors in source/<type>/source.go. Fields of other
// types are forbidden for a type, so a misplaced field is reported instead of ignored.
// Once each source type has its own config struct (feedback point 9), derive this from those structs.
var fieldsBySourceType = map[string]sourceTypeFields{
	"local":  {allowed: []string{"dir", "delimiter", "json_options"}, required: []string{"dir"}},
	"sftp":   {allowed: []string{"host", "port", "username", "key_file", "remote_dir", "delimiter", "json_options"}, required: []string{"host", "username", "key_file"}},
	"sqldb":  {allowed: []string{"connection_string", "staging_dir"}, required: []string{"connection_string"}},
	"luscii": {allowed: []string{"base_url", "api_key", "endpoints", "json_options"}, required: []string{"base_url", "api_key", "endpoints"}},
}

func main() {
	repositoryRoot := flag.String("root", "..", "path to the Fenix repository root")
	outputPath := flag.String("out", "../fenix-config.schema.json", "file to write the schema to")
	flag.Parse()

	if err := writeSchema(*repositoryRoot, *outputPath); err != nil {
		fmt.Fprintln(os.Stderr, "genschema:", err)
		os.Exit(1)
	}
}

// writeSchema builds the config schema and writes it as indented JSON to outputPath.
func writeSchema(repositoryRoot, outputPath string) error {
	absoluteOutputPath, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}
	// The comment reader derives package import paths from file paths relative to the
	// working directory, so it must run from the repository root.
	if err := os.Chdir(repositoryRoot); err != nil {
		return fmt.Errorf("change to repository root: %w", err)
	}

	schema, err := buildSchema()
	if err != nil {
		return err
	}
	content, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(absoluteOutputPath, append(content, '\n'), 0o644)
}

// buildSchema reflects config.Config into a schema and adds what reflection cannot see.
func buildSchema() (*jsonschema.Schema, error) {
	reflector := &jsonschema.Reflector{
		FieldNameTag:               "yaml", // field names in the config file come from the yaml tags
		RequiredFromJSONSchemaTags: true,   // only fields tagged `jsonschema:"required"` are required
		ExpandedStruct:             true,   // Config's fields at the root instead of behind a $ref
		Anonymous:                  true,   // no $id derived from the Go package path
	}
	if err := reflector.AddGoComments(modulePath, "config", jsonschema.WithFullComment()); err != nil {
		return nil, fmt.Errorf("read config comments: %w", err)
	}

	schema := reflector.Reflect(&config.Config{})
	schema.Title = "Fenix-configuratie"
	schema.Description = "Configbestand van Fenix. Beweeg de muis over een veld om te zien wat het doet."

	sourceSchema, found := schema.Definitions["SourceConfig"]
	if !found {
		return nil, fmt.Errorf("SourceConfig definition not found")
	}
	schema.Definitions["JSONFileOptions"] = jsonFileOptionsSchema()
	sourceSchema.Properties.Set("json_options", jsonOptionsSchema())
	sourceSchema.AllOf = sourceTypeRules(sourceSchema)

	addMarkdownDescriptions(schema)
	return schema, nil
}

// jsonOptionsSchema describes the json_options block of a source. The Go field is an
// untyped map[string]interface{}, so reflection cannot see its structure.
func jsonOptionsSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Description: "Optioneel, bij `local`, `sftp` en `luscii`. Per JSON-bestand: hoe de records over tabellen worden verdeeld. " +
			"De sleutel is de bestandsnaam zonder extensie, in kleine letters (`journeys.json` → `journeys`). " +
			"Bij `luscii` is de sleutel de `table` van het endpoint.\n\n" +
			"Voorbeeld:\n```yaml\njson_options:\n  journeys:\n    id_field: id\n    children:\n      steps: {}\n```\n\n" +
			"Bij een `local`-bron `pja` geeft dit de tabellen `pja_journeys` en `pja_journeys_steps`.",
		AdditionalProperties: &jsonschema.Schema{Ref: "#/$defs/JSONFileOptions"},
	}
}

// jsonFileOptionsSchema describes the options for one JSON file or child list.
// It mirrors local.JSONFileConfig and refers to itself for nested children.
func jsonFileOptionsSchema() *jsonschema.Schema {
	properties := jsonschema.NewProperties()
	properties.Set("id_field", &jsonschema.Schema{
		Type: "string",
		Description: "Optioneel, maar nodig als er `children` zijn. Veld waarvan de waarde uniek is per record. " +
			"Het wordt de primaire sleutel van de tabel, en child-rijen bewaren deze waarde om naar hun parent te verwijzen.\n\n" +
			"Voorbeeld: `id_field: id` bij `journeys` → elke rij in `pja_journeys_steps` krijgt een kolom " +
			"`pja_journeys_id` met de `id` van de bijbehorende journey.",
		Examples: []any{"id"},
	})
	properties.Set("children", &jsonschema.Schema{
		Type: "object",
		Description: "Optioneel. Lijsten binnen elk record die een eigen tabel worden, met de veldnaam als sleutel.\n\n" +
			"De child-tabel heet `<parent-tabel>_<veld>`. Gebruik `{}` voor \"eigen tabel, verder geen " +
			"instructies\", of geef opnieuw `id_field` en `children` om nog een niveau dieper te splitsen.\n\n" +
			"Voorbeeld:\n```yaml\nchildren:\n  steps:\n    id_field: id\n    children:\n      completions: {}\n```\n\n" +
			"Een lijst die hier niet genoemd wordt, blijft als JSON-tekst in de parent-rij staan.",
		AdditionalProperties: &jsonschema.Schema{
			AnyOf: []*jsonschema.Schema{
				{Ref: "#/$defs/JSONFileOptions"},
				{Type: "null"}, // an empty value means the same as {}
			},
		},
	})
	return &jsonschema.Schema{
		Type:                 "object",
		Properties:           properties,
		AdditionalProperties: jsonschema.FalseSchema,
		Description: "Hoe één JSON-bestand (of child-lijst) over stagingtabellen wordt verdeeld.\n\n" +
			"`{}` betekent: één tabel, geneste lijsten blijven JSON-tekst.",
	}
}

// sourceTypeRules returns one if/then rule per source type: its required fields, and
// `false` (not allowed) for every field that belongs only to other types.
func sourceTypeRules(sourceSchema *jsonschema.Schema) []*jsonschema.Schema {
	var fieldNames []string
	for property := sourceSchema.Properties.Oldest(); property != nil; property = property.Next() {
		if property.Key != "type" {
			fieldNames = append(fieldNames, property.Key)
		}
	}

	sourceTypes := make([]string, 0, len(fieldsBySourceType))
	for sourceType := range fieldsBySourceType {
		sourceTypes = append(sourceTypes, sourceType)
	}
	sort.Strings(sourceTypes) // stable output, so regenerating gives no spurious diff

	rules := make([]*jsonschema.Schema, 0, len(sourceTypes))
	for _, sourceType := range sourceTypes {
		rules = append(rules, sourceTypeRule(sourceType, fieldsBySourceType[sourceType], fieldNames))
	}
	return rules
}

// sourceTypeRule builds the if/then rule for one source type.
func sourceTypeRule(sourceType string, fields sourceTypeFields, fieldNames []string) *jsonschema.Schema {
	isAllowed := map[string]bool{}
	for _, fieldName := range fields.allowed {
		isAllowed[fieldName] = true
	}

	forbidden := jsonschema.NewProperties()
	for _, fieldName := range fieldNames {
		if !isAllowed[fieldName] {
			forbidden.Set(fieldName, jsonschema.FalseSchema)
		}
	}

	condition := jsonschema.NewProperties()
	condition.Set("type", &jsonschema.Schema{Const: sourceType})
	return &jsonschema.Schema{
		If:   &jsonschema.Schema{Properties: condition, Required: []string{"type"}},
		Then: &jsonschema.Schema{Required: fields.required, Properties: forbidden},
	}
}

// addMarkdownDescriptions copies every description to markdownDescription, which the
// VS Code YAML extension renders as Markdown on hover (so code and examples show nicely).
func addMarkdownDescriptions(schema *jsonschema.Schema) {
	visited := map[*jsonschema.Schema]bool{}
	var visit func(*jsonschema.Schema)
	visit = func(current *jsonschema.Schema) {
		if current == nil || visited[current] {
			return
		}
		visited[current] = true
		if current.Description != "" {
			if current.Extras == nil {
				current.Extras = map[string]any{}
			}
			current.Extras["markdownDescription"] = current.Description
		}
		for _, child := range childSchemas(current) {
			visit(child)
		}
	}
	visit(schema)
}

// childSchemas returns the schemas directly nested in schema that can carry a description.
func childSchemas(schema *jsonschema.Schema) []*jsonschema.Schema {
	children := []*jsonschema.Schema{schema.Items, schema.AdditionalProperties}
	for _, definition := range schema.Definitions {
		children = append(children, definition)
	}
	if schema.Properties != nil {
		for property := schema.Properties.Oldest(); property != nil; property = property.Next() {
			children = append(children, property.Value)
		}
	}
	children = append(children, schema.AnyOf...)
	return append(children, schema.AllOf...)
}
