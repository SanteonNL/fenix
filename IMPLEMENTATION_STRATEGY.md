# Implementation Strategy for Intelligent CSV2FHIR Mapping

## Overview

This document outlines how to integrate intelligent mapping into the existing CSV2FHIR converter to replace hardcoded mappings with auto-derived, convention-based mappings.

---

## Current State Analysis

### What Currently Exists

1. **[converter.go](cmd/csv2fhir/converter/converter.go)**
   - Basic converter with hardcoded mappings for each resource type
   - Manual `convertToPatient()`, `convertToObservation()`, etc. methods
   - Simple column-to-field mapping using if-statements

2. **[loader.go](cmd/csv2fhir/loader/loader.go)**
   - Loads CSV into SQLite tables
   - Creates tables dynamically with all-TEXT columns
   - No column type inference

3. **Configuration**
   - YAML-based configuration
   - `resourceType` specified per conversion
   - Optional `mappings` file support (unused)

### Current Limitations

- ❌ Hardcoded mappings only for 5 resource types
- ❌ No auto-detection from table names
- ❌ No intelligent field derivation
- ❌ No assembly of complex types (CodeableConcept, Quantity)
- ❌ Manual mapping file creation required
- ❌ No code system inference
- ❌ No reference type resolution

---

## Implementation Plan

### Phase 1: Core Intelligent Mapping Engine (DONE ✓)

**Already Created**: `intelligent_mapping.go`

```go
// Core Components
- IntelligentMappingEngine: Main conversion engine
- deriveFieldPath(): SQL column → FHIR path conversion
- inferDataType(): Data type detection
- applyHeuristicRules(): Pattern-based rule engine
- Predefined mappings for Patient, Observation, Condition, Procedure
```

**Status**: ✅ Complete
**Tests**: See INTELLIGENT_MAPPING_EXAMPLES.md

---

### Phase 2: Integration with Existing Converter

#### Step 1: Update `FHIRConverter` struct
```go
// cmd/csv2fhir/converter/converter.go

type FHIRConverter struct {
    db              *sqlx.DB
    resourceType    string
    mappings        map[string]string
    logger          zerolog.Logger
    
    // NEW: Add intelligent mapping engine
    mappingEngine   *IntelligentMappingEngine  // ← NEW
    autoDetect      bool                       // ← NEW
}

// Updated constructor
func NewFHIRConverter(db *sqlx.DB, resourceType string, logger zerolog.Logger) *FHIRConverter {
    // Auto-detect resource type from table name if not specified
    if resourceType == "" {
        resourceType = inferResourceTypeFromTableName(tableName)
    }
    
    return &FHIRConverter{
        db:            db,
        resourceType:  resourceType,
        mappings:      make(map[string]string),
        logger:        logger,
        mappingEngine: NewIntelligentMappingEngine(resourceType), // ← NEW
        autoDetect:    true, // Default to intelligent mapping
    }
}
```

#### Step 2: Add Configuration Options
```yaml
# config/csv2fhir.yaml
fhir:
  resourceType: "" # Leave empty to auto-detect from table name
  autoDetect: true # Use intelligent mapping (if true, ignores hardcoded mappings)
  mappings: ""     # Optional: override intelligent mappings with custom YAML
  inferCodeSystems: true  # Auto-infer code system URIs
  dateFormatHints: {}     # Optional date format overrides
```

#### Step 3: Update ConvertTableToFHIR Method
```go
func (fc *FHIRConverter) ConvertTableToFHIR(tableName string) ([]interface{}, error) {
    // 1. Auto-detect resource type if needed
    if fc.resourceType == "" {
        fc.resourceType = inferResourceTypeFromTableName(tableName)
        if fc.resourceType == "" {
            return nil, fmt.Errorf("cannot infer resource type from table: %s", tableName)
        }
        fc.logger.Info().Str("table", tableName).Str("inferred", fc.resourceType).Msg("Resource type auto-detected")
    }
    
    // 2. Get table columns and metadata
    columns, types, err := fc.getTableMetadata(tableName)
    if err != nil {
        return nil, err
    }
    
    // 3. Set column types in mapping engine for type inference
    fc.mappingEngine.columnTypes = types
    
    // 4. Auto-derive or load mappings
    if fc.autoDetect {
        fc.autoDeriveColumnMappings(columns)
    } else if len(fc.mappings) == 0 {
        fc.loadMappingsFromFile() // Fallback to YAML file
    }
    
    // 5. Query and convert table data
    query := fmt.Sprintf("SELECT * FROM \"%s\"", tableName)
    rows, err := fc.db.Query(query)
    if err != nil {
        return nil, fmt.Errorf("failed to query table: %w", err)
    }
    defer rows.Close()
    
    var resources []interface{}
    for rows.Next() {
        dataMap := fc.scanRow(rows, columns)
        resource := fc.convertToFHIRResource(dataMap)
        if resource != nil {
            resources = append(resources, resource)
        }
    }
    
    return resources, rows.Err()
}

// NEW: Auto-derive column mappings
func (fc *FHIRConverter) autoDeriveColumnMappings(columns []string) {
    for _, col := range columns {
        mapping := fc.mappingEngine.AutoDeriveMapping(col)
        fc.mappings[col] = mapping.FHIRPath
        fc.logger.Debug().
            Str("column", col).
            Str("fhirPath", mapping.FHIRPath).
            Str("dataType", mapping.DataType).
            Msg("Column mapping derived")
    }
}

// NEW: Get table column names and types
func (fc *FHIRConverter) getTableMetadata(tableName string) ([]string, map[string]string, error) {
    query := fmt.Sprintf("PRAGMA table_info(\"%s\")", tableName)
    rows, err := fc.db.Query(query)
    if err != nil {
        return nil, nil, err
    }
    defer rows.Close()
    
    var columns []string
    types := make(map[string]string)
    
    for rows.Next() {
        var cid int
        var name, sqlType string
        var notnull, pk int
        var dfltValue *string
        
        if err := rows.Scan(&cid, &name, &sqlType, &notnull, &dfltValue, &pk); err != nil {
            return nil, nil, err
        }
        
        columns = append(columns, name)
        types[name] = sqlType
    }
    
    return columns, types, rows.Err()
}
```

#### Step 4: Update convertToFHIRResource
```go
func (fc *FHIRConverter) convertToFHIRResource(data map[string]interface{}) interface{} {
    // Create resource structure (could be map or typed struct)
    resource := make(map[string]interface{})
    resource["resourceType"] = fc.resourceType
    
    // Process each column
    for column, value := range data {
        fhirPath, exists := fc.mappings[column]
        if !exists {
            fc.logger.Debug().Str("column", column).Msg("No mapping found, skipping")
            continue
        }
        
        // Transform value based on data type and path
        transformedValue := fc.transformValue(column, value, fhirPath)
        
        // Set in resource using nested field logic
        fc.setNestedField(resource, fhirPath, transformedValue)
    }
    
    return resource
}

// NEW: Transform value based on FHIR type requirements
func (fc *FHIRConverter) transformValue(column, value interface{}, fhirPath string) interface{} {
    if value == nil || value == "" {
        return nil
    }
    
    dataType := fc.mappingEngine.inferDataType(column.(string))
    
    switch dataType {
    case "date":
        dateStr, _ := ConvertToFHIRDate(value, "")
        return dateStr
    case "code":
        codeVal := toString(value)
        codeSystem := InferCodeSystem(column.(string), codeVal)
        return MapValueToCodeableConcept(codeVal, "", codeSystem)
    case "reference":
        return MapValueToReference(fc.inferRefType(column.(string)), toString(value))
    case "boolean":
        return toBoolean(value)
    case "integer":
        return toInt(value)
    case "decimal":
        return toFloat64(value)
    default:
        return toString(value)
    }
}

// NEW: Set nested field in resource (handles arrays, objects)
func (fc *FHIRConverter) setNestedField(resource map[string]interface{}, path string, value interface{}) {
    // Example: "name[0].given[0]" → resource["name"][0]["given"][0]
    // Implementation uses path parser and recursive setting
    parseAndSetPath(resource, path, value)
}

// NEW: Infer reference type from column name
func (fc *FHIRConverter) inferRefType(columnName string) string {
    columnLower := strings.ToLower(columnName)
    
    if strings.Contains(columnLower, "patient") { return "Patient" }
    if strings.Contains(columnLower, "practitioner") || strings.Contains(columnLower, "performer") { return "Practitioner" }
    if strings.Contains(columnLower, "organization") { return "Organization" }
    if strings.Contains(columnLower, "encounter") { return "Encounter" }
    if strings.Contains(columnLower, "location") { return "Location" }
    if strings.Contains(columnLower, "specimen") { return "Specimen" }
    if strings.Contains(columnLower, "device") { return "Device" }
    
    return "Resource"
}
```

---

### Phase 3: Helper Functions

```go
// cmd/csv2fhir/converter/helpers.go (new file)

// Path parser for nested field setting
type FieldPath struct {
    segments []PathSegment
}

type PathSegment struct {
    fieldName string
    indices   []int
}

func (p *FieldPath) Set(resource map[string]interface{}, value interface{}) {
    // Navigate nested structure and set value
    // Handles: name[0].given[0] → nested arrays and objects
}

// FHIR resource builder utilities
func BuildCodeableConcept(code, display, system string) map[string]interface{} {
    cc := make(map[string]interface{})
    
    if display != "" {
        cc["text"] = display
    }
    
    if code != "" {
        coding := map[string]interface{}{
            "code": code,
        }
        if display != "" {
            coding["display"] = display
        }
        if system != "" {
            coding["system"] = system
        }
        cc["coding"] = []interface{}{coding}
    }
    
    return cc
}

func BuildQuantity(value, unit, system, code string) map[string]interface{} {
    q := make(map[string]interface{})
    
    if value != "" {
        q["value"] = value
    }
    if unit != "" {
        q["unit"] = unit
    }
    if system != "" {
        q["system"] = system
    }
    if code != "" {
        q["code"] = code
    }
    
    return q
}

func BuildReference(resourceType, id string) map[string]interface{} {
    return map[string]interface{}{
        "reference": fmt.Sprintf("%s/%s", resourceType, id),
        "type": resourceType,
    }
}

// Type conversion helpers
func toString(value interface{}) string { /* ... */ }
func toInt(value interface{}) int { /* ... */ }
func toFloat64(value interface{}) float64 { /* ... */ }
func toBoolean(value interface{}) bool { /* ... */ }
func toDate(value interface{}, format string) string { /* ... */ }
```

---

### Phase 4: Configuration Loading

```go
// cmd/csv2fhir/config/config.go - Update

type Config struct {
    Database DatabaseConfig
    CSV      CSVConfig
    FHIR     FHIRConfig
    Output   OutputConfig
}

type FHIRConfig struct {
    ResourceType       string                 // Leave empty for auto-detect
    AutoDetect         bool                   // Use intelligent mapping
    Mappings           string                 // Path to custom mapping file
    CodeSystems        map[string]string      // Override code system URIs
    DateFormats        map[string]string      // Column-specific date formats
    InferCodeSystems   bool                   // Auto-infer code systems
    Transformations    map[string]string      // Column transformations (functions)
}

// Loading logic
func (cfg *FHIRConfig) LoadMappingsFromFile(path string) error {
    // Parse YAML/JSON mapping file
    // Return mappings: map[string]string
}
```

---

### Phase 5: Command-Line Integration

```go
// cmd/csv2fhir/main.go - Update

var (
    autoDetect = flag.Bool("auto-detect", true, "Use intelligent mapping (default true)")
    tableName  = flag.String("table", "", "Convert specific table (if omitted, converts all)")
    dryRun     = flag.Bool("dry-run", false, "Show mappings without converting")
)

func main() {
    // ... existing code ...
    
    if *dryRun {
        // Show derived mappings for review
        showMappings(db, *tableName)
    } else {
        convertToFHIR(db, cfg, &log)
    }
}

func showMappings(db *sqlx.DB, tableName string) {
    engine := converter.NewIntelligentMappingEngine(tableName)
    
    columns, types, _ := getTableMetadata(db, tableName)
    
    fmt.Println("\n=== Auto-Derived Mappings ===")
    fmt.Printf("Resource Type: %s\n", tableName)
    fmt.Println("\nColumn → FHIR Path (Type):")
    
    for _, col := range columns {
        engine.columnTypes[col] = types[col]
        mapping := engine.AutoDeriveMapping(col)
        fmt.Printf("  %s → %s (%s)\n", col, mapping.FHIRPath, mapping.DataType)
    }
}
```

---

## Testing Strategy

### Unit Tests

```go
// cmd/csv2fhir/converter/intelligent_mapping_test.go

func TestPatientMappings(t *testing.T) {
    engine := NewIntelligentMappingEngine("Patient")
    
    testCases := []struct {
        column   string
        expected string
    }{
        {"patient_id", "id"},
        {"first_name", "name[0].given[0]"},
        {"last_name", "name[0].family"},
        {"date_of_birth", "birthDate"},
        {"phone_number", "telecom[0].value"},
        {"managing_organization_id", "managingOrganization.reference"},
    }
    
    for _, tc := range testCases {
        mapping := engine.AutoDeriveMapping(tc.column)
        if mapping.FHIRPath != tc.expected {
            t.Errorf("Column %s: expected %s, got %s", 
                tc.column, tc.expected, mapping.FHIRPath)
        }
    }
}

func TestTypeInference(t *testing.T) {
    engine := NewIntelligentMappingEngine("Patient")
    
    testCases := map[string]string{
        "date_of_birth": "date",
        "phone_number": "string",
        "managing_org_id": "reference",
        "active": "boolean",
    }
    
    for col, expected := range testCases {
        dataType := engine.inferDataType(col)
        if dataType != expected {
            t.Errorf("Column %s: expected %s, got %s", col, expected, dataType)
        }
    }
}
```

### Integration Tests

```go
func TestFullConversion(t *testing.T) {
    // 1. Create test table
    // 2. Insert sample data
    // 3. Convert to FHIR
    // 4. Validate output matches expected FHIR structure
}

func TestTableAutoDetection(t *testing.T) {
    // 1. Create tables with different naming conventions
    // 2. Verify resource type detection
    // 3. Verify correct converter is used
}
```

---

## Migration Path

### For Existing Users

1. **Backward Compatibility** ✅
   - Existing hardcoded mappings still work if `autoDetect: false`
   - Custom mapping files still supported
   - No breaking changes

2. **Gradual Migration**
   ```yaml
   # Option 1: Use intelligent mapping (new default)
   fhir:
     autoDetect: true
   
   # Option 2: Keep existing behavior
   fhir:
     autoDetect: false
     mappings: "config/my-mappings.yaml"
   
   # Option 3: Hybrid (override specific columns)
   fhir:
     autoDetect: true
     mappings: "config/my-overrides.yaml"  # Overrides auto-derived
   ```

3. **Dry-Run for Validation**
   ```bash
   # Preview auto-derived mappings before converting
   ./csv2fhir -config config.yaml -dry-run
   
   # Shows:
   # Resource Type: Patient
   # Column → FHIR Path (Type):
   #   patient_id → id (string)
   #   first_name → name[0].given[0] (string)
   #   date_of_birth → birthDate (date)
   # ...
   ```

---

## Benefits of Implementation

| Benefit | Current | After Implementation |
|---|---|---|
| Setup Time | Manual mapping file | Zero (auto-detected) |
| New Resource Type | Hardcode new converter | Add to engine rules |
| Customization | Modify source code | YAML override |
| Maintainability | Spreads logic | Centralized engine |
| Extensibility | Limited patterns | Flexible rules |
| Code Reuse | None | Single engine |
| Scalability | O(n) new types | O(1) new types |

---

## Success Criteria

- ✅ Zero-configuration conversion for standard table names
- ✅ Support all major FHIR resources (Patient, Observation, Condition, Procedure, Organization, Medication)
- ✅ Automatic assembly of complex types (CodeableConcept, Quantity, Reference)
- ✅ Backward compatible with existing configurations
- ✅ Comprehensive logging for mapping derivation
- ✅ Dry-run mode to preview mappings
- ✅ >90% test coverage for mapping engine
- ✅ Documentation and examples for all patterns

---

## Deliverables

- ✅ `intelligent_mapping.go` - Core engine
- ✅ `SQL_TO_FHIR_MAPPING_GUIDE.md` - Comprehensive guide
- ✅ `INTELLIGENT_MAPPING_EXAMPLES.md` - Practical examples
- ✅ `SQL_TO_FHIR_QUICK_REFERENCE.md` - Quick reference card
- ⏳ Integration with converter (Phase 2)
- ⏳ Updated configuration system (Phase 3)
- ⏳ Comprehensive tests (Phase 4)
- ⏳ Updated documentation (Phase 5)

---

## Next Steps

1. **Review** intelligent_mapping.go implementation
2. **Review** documentation and examples
3. **Plan Phase 2** integration with converter
4. **Schedule** integration work
5. **Implement** with TDD approach
6. **Test** thoroughly
7. **Release** with deprecation notice for hardcoded mappings

