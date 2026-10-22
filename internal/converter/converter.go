package converter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// fhirTypeCache caches reflect.Type results keyed by FHIR resource/type name.
// Reflection walks the struct field list every call — for 1000 resources with
// 10 child nodes each that's 10 000 redundant walks on the same types.
// The cache makes each type's reflection work happen exactly once.
var fhirTypeCache sync.Map // string → reflect.Type

// warnLogMu serializes writes of output/warnings.log, since resources are
// converted concurrently (several requests, export batches).
var warnLogMu sync.Mutex

// fhirOutput wraps a validated FHIR struct and ensures "resourceType" appears
// as the first JSON field.
//
// The samply-generated fhir.* models already inject resourceType via a custom
// MarshalJSON — but always as the LAST field (because they embed OtherXxx
// first and declare ResourceType string after it). This wrapper moves it to
// the front so the output is easier to read.
type fhirOutput struct {
	resourceType string
	inner        interface{}
}

// Unwrap returns the resourceType and the underlying resource value for one
// element of the slice ConvertSQL/ExportToNDJSON operate on: a fhirOutput
// wrapper unwraps to its resourceType and inner (the concrete *fhir.* struct
// pointer); a raw map[string]interface{} (a resource type the converter has
// no typed model for, see newFHIRResource) returns its own "resourceType"
// field and itself, with typed=false so callers can tell the two apart —
// notably deident.Deidentify, which refuses to run against an untyped map
// since it can't walk or verify coverage on one.
func Unwrap(v interface{}) (resourceType string, resource interface{}, typed bool) {
	switch r := v.(type) {
	case fhirOutput:
		return r.resourceType, r.inner, true
	case map[string]interface{}:
		rt, _ := r["resourceType"].(string)
		return rt, r, false
	default:
		return "", v, false
	}
}

func (f fhirOutput) MarshalJSON() ([]byte, error) {
	inner, err := json.Marshal(f.inner)
	if err != nil {
		return nil, err
	}
	rt, err := json.Marshal(f.resourceType)
	if err != nil {
		return nil, err
	}

	// The generated models append: ,"resourceType":"Patient"}
	// Strip it from the end so we can re-inject it at the front.
	suffix := make([]byte, 0, len(rt)+18)
	suffix = append(suffix, `,"resourceType":`...)
	suffix = append(suffix, rt...)
	suffix = append(suffix, '}')

	if bytes.HasSuffix(inner, suffix) {
		// Remove the trailing ,"resourceType":"X"} and re-add closing brace
		trimmed := append(inner[:len(inner)-len(suffix)], '}')
		if bytes.Equal(trimmed, []byte("{}")) {
			return append(append([]byte(`{"resourceType":`), rt...), '}'), nil
		}
		out := make([]byte, 0, len(trimmed)+len(rt)+20)
		out = append(out, `{"resourceType":`...)
		out = append(out, rt...)
		out = append(out, ',')
		out = append(out, trimmed[1:]...) // skip opening '{'
		return out, nil
	}

	// Fallback: inner has no resourceType (e.g. raw map) — inject at front naively.
	if len(inner) >= 2 && inner[0] == '{' && inner[1] == '}' {
		return append(append([]byte(`{"resourceType":`), rt...), '}'), nil
	}
	out := make([]byte, 0, len(inner)+len(rt)+20)
	out = append(out, `{"resourceType":`...)
	out = append(out, rt...)
	out = append(out, ',')
	out = append(out, inner[1:]...)
	return out, nil
}

// RowData represents one SQL row at a given FHIR path level.
// Multiple rows at the same fhir_path + parent_id become FHIR array elements.
type RowData struct {
	ID       string
	ParentID string
	Data     map[string]interface{} // leaf field name → value (no [n] notation)
}

// ResourceResult groups RowData by FHIR path (e.g. "Patient", "Patient.name", "Patient.name.coding")
type ResourceResult map[string][]RowData

// FHIRConverter reads SQL rows and converts them to FHIR resources.
//
// SQL row format (columns):
//
//	resource_id  – identifier of the root resource (e.g. patient id)
//	id           – identifier of this specific row
//	parent_id    – id of the parent row (empty string for root rows)
//	fhir_path    – FHIR path at this level, e.g. "Patient", "Patient.name", "Patient.name.coding"
//	<field>      – any other column is a leaf field value at this path level.
//	               Dot-notation is allowed for simple scalar nesting (e.g. "subject.reference")
//	               Arrays are created by providing multiple rows with the same fhir_path + parent_id.
type FHIRConverter struct {
	db         *sqlx.DB
	logger     zerolog.Logger
	profile    *ProfileService
	conceptMap *ConceptMapService

	// statementWorkers bounds how many statements of one ConvertSQL call run
	// at the same time (each holds one database connection while reading).
	// 1 (the default) runs them one after another; raise it only when db is a
	// real connection pool (see config.ApplyPool) — an in-memory SQLite
	// database gives every extra connection its own, empty, database.
	statementWorkers int
	// buildWorkers bounds the goroutines that turn grouped rows into
	// validated FHIR resources (CPU only, no database access). 0 means GOMAXPROCS.
	buildWorkers int
}

// NewFHIRConverter creates a new converter.
func NewFHIRConverter(db *sqlx.DB, logger zerolog.Logger, profile *ProfileService, conceptMap *ConceptMapService) *FHIRConverter {
	return &FHIRConverter{db: db, logger: logger, profile: profile, conceptMap: conceptMap, statementWorkers: 1}
}

// SetConcurrency tunes ConvertSQL's parallelism: statements is how many SQL
// statements may run concurrently (needs a pooled db, see FHIRConverter), build
// how many goroutines build and validate resources (0 = GOMAXPROCS).
// Call it before the converter is shared between goroutines.
func (fc *FHIRConverter) SetConcurrency(statements, build int) {
	if statements < 1 {
		statements = 1
	}
	fc.statementWorkers = statements
	fc.buildWorkers = build
}

func (fc *FHIRConverter) buildWorkerCount() int {
	if fc.buildWorkers > 0 {
		return fc.buildWorkers
	}
	return runtime.GOMAXPROCS(0)
}

// ConvertSQL executes a SQL string that may contain multiple statements separated by ";".
// Each statement should return rows with: resource_id, id, parent_id, fhir_path, + field columns.
// Multiple rows at the same fhir_path + parent_id become array elements in the FHIR resource.
// Resources that fail FHIR struct validation are logged and skipped.
func (fc *FHIRConverter) ConvertSQL(query string) ([]interface{}, error) {
	return fc.ConvertSQLContext(context.Background(), query)
}

// ConvertSQLContext is ConvertSQL with a context that cancels the running
// statements (e.g. when the HTTP client goes away). The returned resources are
// sorted by resource_id, so the output does not depend on goroutine timing.
func (fc *FHIRConverter) ConvertSQLContext(ctx context.Context, query string) ([]interface{}, error) {
	fc.logger.Info().Msg("Converting SQL to FHIR")

	stmts := SplitStatements(query)
	results := make([]*stmtResult, len(stmts))

	var wg sync.WaitGroup
	sem := make(chan struct{}, fc.statementWorkers)
	for i, stmt := range stmts {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int, stmt string) {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := fc.executeStatement(ctx, stmt, i+1)
			if err != nil {
				fc.logger.Error().Err(err).Int("statement", i+1).Msg("Statement failed, continuing")
			}
			results[i] = res
		}(i, stmt)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Merge in statement order so array elements and the root path of a
	// resource come out the same as when the statements ran sequentially.
	resources := make(map[string]ResourceResult)
	rootPaths := make(map[string]string)
	for _, r := range results {
		if r != nil {
			r.mergeInto(resources, rootPaths)
		}
	}

	return fc.buildResources(resources, rootPaths), nil
}

// SourceCodes reverse-maps a FHIR search code for resourceType.field (e.g.
// Observation.code) through the profile's valueset binding and its concept
// map(s) to the source codes stored in the database — see
// ConceptMapService.ReverseTranslate for the meaning of exact and mapped.
func (fc *FHIRConverter) SourceCodes(resourceType, field, targetCode string) (codes []string, exact, mapped bool) {
	if fc.profile == nil || fc.conceptMap == nil {
		return nil, true, false
	}
	vsURI := fc.profile.ValuesetURI(resourceType + "." + field)
	if vsURI == "" {
		return nil, true, false
	}
	return fc.conceptMap.ReverseTranslate(vsURI, targetCode)
}

// QueryStrings runs query and returns the first column of every row as a
// string — used for single-column id lists such as a Group's members query.
func (fc *FHIRConverter) QueryStrings(query string) ([]string, error) {
	return fc.QueryStringsContext(context.Background(), query)
}

// QueryStringsContext is QueryStrings with a cancellable context.
func (fc *FHIRConverter) QueryStringsContext(ctx context.Context, query string) ([]string, error) {
	rows, err := fc.db.QueryxContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var v interface{}
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		if v != nil {
			out = append(out, toString(v))
		}
	}
	return out, rows.Err()
}

// QueryRowsContext runs query and returns every column of every row as
// strings (NULL becomes "") — used for the patient-number query (number,
// replaced_by, optionally test_patient).
func (fc *FHIRConverter) QueryRowsContext(ctx context.Context, query string) ([][]string, error) {
	rows, err := fc.db.QueryxContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out [][]string
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			if v != nil {
				row[i] = toString(v)
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// stmtResult is the rows of one statement, grouped by resource. Statements
// are collected separately (so they can run in parallel) and merged afterwards.
type stmtResult struct {
	resources map[string]ResourceResult
	rootPaths map[string]string
}

// mergeInto adds r's rows to resources/rootPaths. Merging statements in their
// original order reproduces what a sequential run would have produced.
func (r *stmtResult) mergeInto(resources map[string]ResourceResult, rootPaths map[string]string) {
	for resourceID, byPath := range r.resources {
		dst := resources[resourceID]
		if dst == nil {
			// Nothing merged for this resource yet: adopt the map as is.
			resources[resourceID] = byPath
			continue
		}
		for path, rows := range byPath {
			dst[path] = append(dst[path], rows...)
		}
	}
	for resourceID, path := range r.rootPaths {
		if rootPaths[resourceID] == "" {
			rootPaths[resourceID] = path
		}
	}
}

// Which of the four metadata columns (if any) a result column is.
const (
	colData = iota
	colResourceID
	colID
	colParentID
	colFHIRPath
)

func classifyColumn(name string) int {
	switch strings.ToLower(name) {
	case "resource_id":
		return colResourceID
	case "id":
		return colID
	case "parent_id":
		return colParentID
	case "fhir_path":
		return colFHIRPath
	}
	return colData
}

// executeStatement runs one SQL statement and groups its rows by resource.
// The partial result is returned even on error (rows read before a mid-stream
// failure are kept, as they were when statements fed shared maps directly).
func (fc *FHIRConverter) executeStatement(ctx context.Context, stmt string, n int) (*stmtResult, error) {
	rows, err := fc.db.QueryxContext(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("statement %d: %w", n, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("statement %d: %w", n, err)
	}
	kinds := make([]int, len(cols))
	for i, c := range cols {
		kinds[i] = classifyColumn(c)
	}
	// The values are scanned into reusable slots; database/sql copies []byte
	// for *interface{} destinations, so keeping them in the row maps is safe.
	vals := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	res := &stmtResult{resources: make(map[string]ResourceResult), rootPaths: make(map[string]string)}
	count := 0
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			fc.logger.Error().Err(err).Msg("Row scan failed")
			continue
		}
		res.addRow(cols, kinds, vals)
		count++
	}
	fc.logger.Debug().Int("statement", n).Int("rows", count).Msg("Statement executed")
	return res, rows.Err()
}

// buildResources converts the grouped ResourceResult map into validated FHIR structs.
// Each resource is first built as a map, then validated by unmarshaling into the
// matching fhir.* struct. Invalid resources are logged and skipped.
//
// Resources are independent of each other, so they are built by a pool of
// workers; the output is ordered by resource id.
func (fc *FHIRConverter) buildResources(resources map[string]ResourceResult, rootPaths map[string]string) []interface{} {
	ids := make([]string, 0, len(resources))
	for id := range resources {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	type built struct {
		out     interface{} // nil when the resource was skipped
		warning string      // validation error message, if that is why it was skipped
	}
	outs := make([]built, len(ids))

	var next atomic.Int64
	var wg sync.WaitGroup
	for w := fc.buildWorkerCount(); w > 0; w-- {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= len(ids) {
					return
				}
				outs[i].out, outs[i].warning = fc.buildResource(ids[i], resources[ids[i]], rootPaths)
			}
		}()
	}
	wg.Wait()

	result := make([]interface{}, 0, len(ids))
	warningCounts := map[string]int{}
	warningDetails := map[string][]string{} // resourceIDs per warning
	for i, b := range outs {
		if b.out != nil {
			result = append(result, b.out)
		}
		if b.warning != "" {
			warningCounts[b.warning]++
			warningDetails[b.warning] = append(warningDetails[b.warning], ids[i])
		}
	}
	fc.logger.Info().Int("resources", len(result)).Msg("Conversion completed")

	// Write warnings summary to a separate log file
	if len(warningCounts) > 0 {
		warnLogPath := "output/warnings.log"
		var sb strings.Builder
		sb.WriteString("Validation Warnings Summary\n==========================\n")
		for msg, count := range warningCounts {
			sb.WriteString(fmt.Sprintf("%d\tx: %s\n", count, msg))
			sb.WriteString("  ResourceIDs: ")
			sb.WriteString(strings.Join(warningDetails[msg], ", "))
			sb.WriteString("\n\n")
		}
		warnLogMu.Lock()
		_ = os.MkdirAll("output", 0755)
		_ = os.WriteFile(warnLogPath, []byte(sb.String()), 0644)
		warnLogMu.Unlock()
		fc.logger.Info().Str("file", warnLogPath).Msg("Wrote validation warnings summary")
	}
	return result
}

// SplitStatements splits a SQL string on ";" into individual non-empty statements.
// Lines that consist only of comments are skipped.
func SplitStatements(sql string) []string {
	var statements []string
	for _, raw := range strings.Split(sql, ";") {
		var lines []string
		for _, line := range strings.Split(raw, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
				lines = append(lines, line)
			}
		}
		stmt := strings.TrimSpace(strings.Join(lines, "\n"))
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}
	return statements
}

// addRow places one scanned SQL row into the correct ResourceResult bucket.
// kinds[i] says whether column i is one of the four metadata columns.
func (r *stmtResult) addRow(cols []string, kinds []int, vals []interface{}) {
	var resourceID, id, parentID, fhirPath string
	for i, k := range kinds {
		switch k {
		case colResourceID:
			resourceID = toString(vals[i])
		case colID:
			id = toString(vals[i])
		case colParentID:
			parentID = toString(vals[i])
		case colFHIRPath:
			fhirPath = toString(vals[i])
		}
	}

	if resourceID == "" || fhirPath == "" {
		return
	}

	byPath := r.resources[resourceID]
	if byPath == nil {
		byPath = make(ResourceResult)
		r.resources[resourceID] = byPath
	}

	// The root path has no dot and no existing root registered
	if !strings.Contains(fhirPath, ".") && r.rootPaths[resourceID] == "" {
		r.rootPaths[resourceID] = fhirPath
	}

	// Collect all data columns (skip the four metadata columns)
	data := make(map[string]interface{}, len(cols))
	for i, k := range kinds {
		if k == colData && vals[i] != nil {
			data[cols[i]] = vals[i]
		}
	}

	byPath[fhirPath] = append(byPath[fhirPath], RowData{ID: id, ParentID: parentID, Data: data})
}

// buildResource turns one resource's grouped rows into a validated FHIR
// resource. out is nil when the resource is skipped; warning is then the
// validation error when that was the reason (a build failure is only logged).
func (fc *FHIRConverter) buildResource(resourceID string, resResult ResourceResult, rootPaths map[string]string) (out interface{}, warning string) {
	rootPath, ok := rootPaths[resourceID]
	if !ok {
		fc.logger.Warn().Str("resourceID", resourceID).Msg("No root fhir_path, skipping")
		return nil, ""
	}

	raw, err := buildFHIRResource(resResult, rootPath)
	if err != nil {
		fc.logger.Error().Err(err).Str("resourceID", resourceID).Msg("Build failed")
		return nil, ""
	}

	// Pretty-printing every resource is expensive, so only do it when the
	// debug line below will actually be written.
	if fc.logger.GetLevel() <= zerolog.DebugLevel {
		rawJSON, _ := json.MarshalIndent(raw, "", "  ")
		fc.logger.Debug().RawJSON("raw", rawJSON).Str("resourceID", resourceID).Msg("Built FHIR resource")
	}

	// Apply concept mappings driven by FHIR profile bindings
	applyConceptMappings(raw, rootPath, fc.profile, fc.conceptMap)

	// Validate by round-tripping through the typed fhir.* struct.
	validated, err := validateThroughStruct(raw, fc.logger)
	if err != nil {
		// raw is logged as it stands after concept mapping and coercion — the
		// state that failed validation.
		rawJSON, _ := json.Marshal(raw)
		fc.logger.Warn().
			Err(err).
			Str("resourceID", resourceID).
			Str("resourceType", rootPath).
			RawJSON("raw", rawJSON).
			Msg("FHIR validation failed — resource skipped")
		return nil, err.Error()
	}

	// Typed structs lack a ResourceType field — wrap so it appears first in JSON.
	// Raw maps (unknown resource types) already carry "resourceType".
	if _, isMap := validated.(map[string]interface{}); isMap {
		return validated, ""
	}
	return fhirOutput{resourceType: rootPath, inner: validated}, ""
}

// buildFHIRResource converts a ResourceResult into a nested FHIR map.
func buildFHIRResource(result ResourceResult, rootPath string) (map[string]interface{}, error) {
	rootRows := result[rootPath]
	if len(rootRows) == 0 {
		return nil, fmt.Errorf("no root rows for path %s", rootPath)
	}

	resource := make(map[string]interface{})
	resource["resourceType"] = rootPath // e.g. "Patient"

	rootRow := rootRows[0]
	// The SQL row format's own "id" column (see package doc) is metadata for
	// linking parent/child rows (processRow strips it out of Data for that
	// reason) — but it's also the resource's own FHIR .id, the same value
	// every other resource's "subject.reference" etc. already points at via
	// 'Patient/' || patient_id. Without this, every resource FENIX produces
	// has no .id at all, which both contradicts the de-identification
	// ruleset (Patient.id/Observation.id both have explicit rules — see
	// config/deident/sim-demo-ruleset.json) and breaks anything trying to
	// correlate a resource back to its own id (e.g. Group export scoping).
	resource["id"] = rootRow.ID

	// Set leaf fields from root row; support dot-notation for simple scalar nesting
	for k, v := range rootRow.Data {
		setNestedValue(resource, k, v)
	}

	// Build array field map from the FHIR struct definition
	arrayFields := getArrayFieldsForType(rootPath)

	// Recursively add child paths
	setChildren(resource, result, rootPath, rootRow.ID, arrayFields)

	return resource, nil
}

// setChildren finds all child paths of parentPath and populates them. This
// normally means direct children (one dot-segment below parentPath), but a
// query can also skip a level entirely — e.g. giving fhir_path
// "Observation.code.coding" with parent_id set to the root Observation's own
// id, never providing an "Observation.code" row of its own. That's a common,
// intentional pattern for a CodeableConcept that carries nothing but its
// coding array (see queries/hix/fhir/observation.sql's "Code" statement) —
// setChildren synthesizes the missing intermediate object rather than
// silently dropping that data (see hasDescendantRows below).
//
// Multiple RowData at the same child path with the same parentID become a
// FHIR array. Fields that are defined as array types in the FHIR struct are
// wrapped in arrays even when there's only one element.
func setChildren(parent map[string]interface{}, result ResourceResult, parentPath string, parentID string, arrayFields map[string]bool) {
	for _, fieldName := range directDescendantSegments(result, parentPath) {
		childPath := parentPath + "." + fieldName
		shouldBeArray := arrayFields[fieldName]

		// Rows given directly at this exact path, belonging to this parent.
		var matching []RowData
		for _, row := range result[childPath] {
			if row.ParentID == parentID {
				matching = append(matching, row)
			}
		}

		if len(matching) > 0 {
			if len(matching) == 1 && !shouldBeArray {
				// Single element and not defined as array in struct → store as single object
				parent[fieldName] = buildChild(matching[0], result, childPath, arrayFields)
			} else {
				// Multiple elements OR field is defined as array in struct → store as array
				arr := make([]interface{}, len(matching))
				for i, row := range matching {
					arr[i] = buildChild(row, result, childPath, arrayFields)
				}
				parent[fieldName] = arr
			}
			continue
		}

		// No rows at exactly childPath — but a deeper path may still carry
		// rows parented directly to this level's id (the skipped-level
		// pattern described above). Synthesize the intermediate object.
		if !hasDescendantRows(result, childPath, parentID) {
			continue
		}
		obj := make(map[string]interface{})
		nestedArrayFields := getArrayFieldsForType(getTypeNameFromPath(childPath))
		setChildren(obj, result, childPath, parentID, nestedArrayFields)
		normalizeObjectArrayFields(obj, getTypeNameFromPath(childPath))
		if len(obj) == 0 {
			continue
		}
		if shouldBeArray {
			parent[fieldName] = []interface{}{obj}
		} else {
			parent[fieldName] = obj
		}
	}
}

// directDescendantSegments returns the distinct immediate next-segments
// (e.g. "code" from "Observation.code" or "Observation.code.coding") among
// every path in result that falls under parentPath, however deep.
func directDescendantSegments(result ResourceResult, parentPath string) []string {
	prefix := parentPath + "."
	seen := map[string]bool{}
	var segments []string
	for path := range result {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		rest := path[len(prefix):]
		segment := rest
		if i := strings.Index(rest, "."); i >= 0 {
			segment = rest[:i]
		}
		if !seen[segment] {
			seen[segment] = true
			segments = append(segments, segment)
		}
	}
	return segments
}

// hasDescendantRows reports whether any path strictly beneath prefix (not
// prefix itself — that's the direct-row case, checked separately) has a row
// whose ParentID matches parentID.
func hasDescendantRows(result ResourceResult, prefix string, parentID string) bool {
	descendantPrefix := prefix + "."
	for path, rows := range result {
		if !strings.HasPrefix(path, descendantPrefix) {
			continue
		}
		for _, row := range rows {
			if row.ParentID == parentID {
				return true
			}
		}
	}
	return false
}

// buildChild converts one RowData into either a scalar, or a nested map with children.
func buildChild(row RowData, result ResourceResult, path string, arrayFields map[string]bool) interface{} {
	// Check whether this path has sub-paths
	hasChildren := false
	for p := range result {
		if isDirectChild(path, p) {
			hasChildren = true
			break
		}
	}

	// Single scalar field, no children → return bare value
	if !hasChildren && len(row.Data) == 1 {
		for _, v := range row.Data {
			return v
		}
	}

	obj := make(map[string]interface{})
	for k, v := range row.Data {
		setNestedValue(obj, k, v) // support dot-notation in data columns
	}

	// Get array fields for the nested type (e.g., if path is "Patient.name", get HumanName array fields)
	nestedArrayFields := getArrayFieldsForType(getTypeNameFromPath(path))
	setChildren(obj, result, path, row.ID, nestedArrayFields)

	// Wrap scalar values in arrays where the struct field expects an array
	normalizeObjectArrayFields(obj, getTypeNameFromPath(path))

	return obj
}

// setNestedValue assigns a value at a dot-separated path inside a map.
// e.g. setNestedValue(m, "subject.reference", "Patient/123")
// produces m["subject"]["reference"] = "Patient/123"
func setNestedValue(obj map[string]interface{}, dotPath string, value interface{}) {
	parts := strings.SplitN(dotPath, ".", 2)
	if len(parts) == 1 {
		obj[dotPath] = value
		return
	}
	key := parts[0]
	child, ok := obj[key].(map[string]interface{})
	if !ok {
		child = make(map[string]interface{})
		obj[key] = child
	}
	setNestedValue(child, parts[1], value)
}

// isDirectChild returns true when child is exactly one level below parent.
func isDirectChild(parent, child string) bool {
	prefix := parent + "."
	if !strings.HasPrefix(child, prefix) {
		return false
	}
	rest := child[len(prefix):]
	return !strings.Contains(rest, ".")
}

// toString converts an interface{} to string, returning "" for nil.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprint(v)
}

// ExportToNDJSON marshals resources to newline-delimited JSON.
func ExportToNDJSON(resources []interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := WriteNDJSON(&buf, resources); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteNDJSON writes resources to w as newline-delimited JSON, one resource
// per line, without building the whole output in memory first.
func WriteNDJSON(w io.Writer, resources []interface{}) error {
	for _, r := range resources {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		b = append(b, '\n')
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}

// ExportToPretty marshals resources to pretty-printed JSON blocks separated by
// a blank line — not valid NDJSON, but easy to read for debugging purposes.
func ExportToPretty(resources []interface{}) ([]byte, error) {
	var out []byte
	for i, r := range resources {
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
		out = append(out, '\n')
		if i < len(resources)-1 {
			out = append(out, '\n') // blank line between resources
		}
	}
	return out, nil
}

// ExportToJSON marshals resources to a pretty-printed JSON array.
func ExportToJSON(resources []interface{}) ([]byte, error) {
	return json.MarshalIndent(resources, "", "  ")
}

// cachedFHIRType returns the reflect.Type for a FHIR type name, using a
// package-level cache so each type's struct fields are inspected only once.
func cachedFHIRType(typeName string) (reflect.Type, bool) {
	if v, ok := fhirTypeCache.Load(typeName); ok {
		return v.(reflect.Type), true
	}
	target, err := newFHIRResource(typeName)
	if err != nil {
		// Unknown type — also try findFHIRType for complex sub-types (HumanName etc.)
		t := findFHIRType(typeName)
		if t == nil {
			return nil, false
		}
		fhirTypeCache.Store(typeName, t)
		return t, true
	}
	t := reflect.TypeOf(target)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	fhirTypeCache.Store(typeName, t)
	return t, true
}

// arrayFieldsCache caches getArrayFieldsForType per type name. The returned
// maps are shared between goroutines and must be treated as read-only.
var arrayFieldsCache sync.Map // string → map[string]bool

// getArrayFieldsForType inspects the FHIR struct and returns which fields are arrays
// by looking at the struct definition and JSON tags. The result is cached and
// shared: callers must not modify it.
func getArrayFieldsForType(resourceType string) map[string]bool {
	if v, ok := arrayFieldsCache.Load(resourceType); ok {
		return v.(map[string]bool)
	}
	fields := computeArrayFields(resourceType)
	arrayFieldsCache.Store(resourceType, fields)
	return fields
}

func computeArrayFields(resourceType string) map[string]bool {
	arrayFields := make(map[string]bool)

	t, ok := cachedFHIRType(resourceType)
	if !ok {
		return arrayFields
	}

	// Iterate through struct fields
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Check if field is a slice/array
		if field.Type.Kind() == reflect.Slice {
			// Get the JSON tag name
			jsonTag := field.Tag.Get("json")
			if jsonTag != "" {
				// Extract the field name (before comma)
				fieldName := strings.Split(jsonTag, ",")[0]
				if fieldName != "" && fieldName != "-" {
					arrayFields[fieldName] = true
				}
			}
		}
	}

	return arrayFields
}

// getTypeNameFromPath extracts the type name from a FHIR path by inspecting struct definitions
// e.g., "Patient.name" -> "HumanName", "Patient.address" -> "Address"
// typeNameCache memoizes getTypeNameFromPath / getTypeNameForField. Both walk the
// struct with reflection (FieldByNameFunc is quadratic in the field count) yet
// depend only on their string arguments, and they run for every nested row.
var typeNameCache sync.Map // string → string

func getTypeNameFromPath(path string) string {
	if v, ok := typeNameCache.Load("p:" + path); ok {
		return v.(string)
	}
	name := computeTypeNameFromPath(path)
	typeNameCache.Store("p:"+path, name)
	return name
}

func computeTypeNameFromPath(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return ""
	}

	// Get the resource type (first part)
	resourceType := parts[0]

	// Get the field name (last part)
	fieldName := parts[len(parts)-1]

	t, ok := cachedFHIRType(resourceType)
	if !ok {
		return ""
	}

	// Find the struct field
	field, ok := t.FieldByNameFunc(func(name string) bool {
		// Match JSON tag to field name
		if f, found := t.FieldByName(name); found {
			jsonTag := f.Tag.Get("json")
			tagName := strings.Split(jsonTag, ",")[0]
			return tagName == fieldName
		}
		return false
	})

	if ok && field.Type.Kind() == reflect.Slice {
		// Get the element type name
		elemType := field.Type.Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		return elemType.Name()
	}

	// Fallback: capitalize the field name
	if len(fieldName) > 0 {
		return strings.ToUpper(fieldName[:1]) + fieldName[1:]
	}
	return ""
}

// normalizeObjectArrayFields wraps scalar values in arrays where the struct definition expects arrays
// This handles nested types like Address.line which should be []string but comes in as string
func normalizeObjectArrayFields(obj map[string]interface{}, typeName string) {
	// Get array fields for this type
	arrayFields := getArrayFieldsForType(typeName)

	// Wrap scalar values in arrays
	for fieldName, shouldBeArray := range arrayFields {
		if !shouldBeArray {
			continue
		}

		val, ok := obj[fieldName]
		if !ok || val == nil {
			continue
		}

		// Check if it's already an array
		if _, isArray := val.([]interface{}); isArray {
			// Already an array, recurse into elements
			for _, elem := range val.([]interface{}) {
				if m, ok := elem.(map[string]interface{}); ok {
					// Recursively normalize nested objects
					normalizeObjectArrayFields(m, getTypeNameForField(typeName, fieldName))
				}
			}
			continue
		}

		// Scalar value - wrap in array
		if _, isMap := val.(map[string]interface{}); isMap {
			// It's an object, wrap it
			obj[fieldName] = []interface{}{val}
			// Recursively normalize the nested object
			if m, ok := val.(map[string]interface{}); ok {
				normalizeObjectArrayFields(m, getTypeNameForField(typeName, fieldName))
			}
		} else {
			// Scalar (string, number, bool) - wrap in array
			obj[fieldName] = []interface{}{val}
		}
	}

	// Recursively process nested objects
	for k, v := range obj {
		if m, ok := v.(map[string]interface{}); ok {
			// This is a nested object - determine its type and normalize it
			nestedType := getTypeNameForField(typeName, k)
			if nestedType != "" {
				normalizeObjectArrayFields(m, nestedType)
			}
		} else if arr, ok := v.([]interface{}); ok {
			// Array of objects - normalize each element
			for _, elem := range arr {
				if m, ok := elem.(map[string]interface{}); ok {
					nestedType := getTypeNameForField(typeName, k)
					if nestedType != "" {
						normalizeObjectArrayFields(m, nestedType)
					}
				}
			}
		}
	}
}

// getTypeNameForField returns the type of a field in a given struct
func getTypeNameForField(structName, fieldName string) string {
	key := "f:" + structName + "." + fieldName
	if v, ok := typeNameCache.Load(key); ok {
		return v.(string)
	}
	name := computeTypeNameForField(structName, fieldName)
	typeNameCache.Store(key, name)
	return name
}

func computeTypeNameForField(structName, fieldName string) string {
	t, ok := cachedFHIRType(structName)
	if !ok {
		return ""
	}

	// Find the struct field by JSON tag
	field, ok := t.FieldByNameFunc(func(name string) bool {
		if f, found := t.FieldByName(name); found {
			jsonTag := f.Tag.Get("json")
			tagName := strings.Split(jsonTag, ",")[0]
			return tagName == fieldName
		}
		return false
	})

	if ok {
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Slice {
			fieldType = fieldType.Elem()
			if fieldType.Kind() == reflect.Ptr {
				fieldType = fieldType.Elem()
			}
		}
		return fieldType.Name()
	}

	return ""
}

// newFHIRResource is imported from validate.go - defined there
