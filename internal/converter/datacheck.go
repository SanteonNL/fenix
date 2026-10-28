package converter

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/SanteonNL/fenix/internal/findings"
)

// SetFindings makes the converter report data-quality findings for every
// resource it builds (see internal/findings). Reporting is log-only: it never
// changes or drops a resource. Call it before the converter is shared between
// goroutines; a nil collector (the default) turns reporting off.
func (fc *FHIRConverter) SetFindings(c *findings.Collector) { fc.findings = c }

// FlushFindings writes everything collected so far to dir as
// findings-<runID>.jsonl and clears the collector. It returns "" when there was
// nothing to write or reporting is off.
func (fc *FHIRConverter) FlushFindings(dir, runID, source, query string) (string, error) {
	if fc.findings == nil || fc.findings.Empty() {
		return "", nil
	}
	path, err := findings.WriteJSONL(dir, runID, fc.findings.Drain(runID, source, query, time.Now().UTC()))
	if err != nil {
		return "", err
	}
	if path != "" {
		fc.logger.Info().Str("file", path).Msg("Wrote data findings")
	}
	return path, nil
}

// reportFindings checks one built resource and records what is wrong with it:
// the typed-model error that made validateThroughStruct reject it (base types
// and codes), and how it conforms to the loaded profile. raw is the resource
// after validateThroughStruct, i.e. coerced and with single values wrapped in
// arrays where the model expects them.
func (fc *FHIRConverter) reportFindings(resourceID, rootPath string, raw map[string]interface{}, structErr error, unmapped ...unmappedCode) {
	if fc.findings == nil {
		return
	}
	fc.findings.ResourceDone(rootPath, structErr != nil)

	profileURL := ""
	if fc.profile != nil {
		if r, ok := fc.profile.Element(rootPath); ok {
			profileURL = r.ProfileURL
		}
	}

	// A rule that fires many times inside one resource (every item of an array)
	// counts once, so a finding's count is the number of resources affected.
	reported := map[string]bool{}
	var asJSON []byte // the resource, serialised once, and only if some finding keeps it
	report := func(sev findings.Severity, code, path, msg, value string) {
		k := code + "|" + path + "|" + msg
		if reported[k] {
			return
		}
		reported[k] = true
		if fc.findings.NeedsSample(code, rootPath, profileURL, path, msg) {
			if asJSON == nil {
				asJSON, _ = json.Marshal(raw)
			}
			fc.findings.AddSample(sev, code, rootPath, profileURL, path, msg, resourceID, value, asJSON)
			return
		}
		fc.findings.Add(sev, code, rootPath, profileURL, path, msg, resourceID, value)
	}

	if structErr != nil {
		reportStructErrors(rootPath, raw, structErr, fc.profile, report)
	}

	// A source code with no mapping is not invalid, but the data is not what the map intends: a warning.
	for _, u := range unmapped {
		rel := strings.TrimPrefix(u.path, rootPath+".")
		path := rootPath
		if rel != u.path {
			if fc.profile != nil {
				rel = fc.profile.ChoicePath(rootPath, rel)
			}
			path += "." + rel
		}
		if u.code == "" {
			u.code = "(empty)"
		}
		if u.status == MapDefault {
			report(findings.Warning, findings.CodeDataUnmapped, path, fmt.Sprintf("no mapping for this source code; the concept map's default (%q) replaces it", u.target), u.code)
		} else {
			report(findings.Warning, findings.CodeDataUnmappedNone, path, "no mapping for this source code and the concept map has no default; it is passed through unchanged", u.code)
		}
	}

	// An Observation value has to fit the ObservationDefinition of its code: right kind of value, and for a
	// coded one a valid answer for that code. The finding names the combination, e.g. "MCD_L: P1".
	if fc.obsDefs != nil && rootPath == "Observation" {
		valueKey, valueType, answers := observationValue(raw)
		for _, v := range fc.obsDefs.Check(observationCodes(raw), valueType, answers) {
			if v.Kind == ObsViolationType {
				report(findings.Warning, findings.CodeDataObsType, rootPath+"."+fc.elementPath(rootPath, valueKey),
					"value type is not permitted for this observation code (ObservationDefinition)", v.ObsCode+": "+v.Detail)
			} else {
				report(findings.Warning, findings.CodeDataObsAnswer, rootPath+"."+fc.elementPath(rootPath, valueKey+".coding.code"),
					"answer is not one of the valid answers for this observation code (ObservationDefinition) and has no mapping", v.ObsCode+": "+v.Detail)
			}
		}
	}

	if fc.profile != nil && fc.profile.HasProfileFor(rootPath) {
		fc.checkProfile(rootPath, raw, report)
	}
}

// maxStructErrors bounds how many bad fields of one resource are reported.
const maxStructErrors = 25

// reportStructErrors reports every field the typed model rejects. The decoder
// stops at the first bad value, so after each error the offending field is
// removed from a copy and the copy is validated again until it passes.
func reportStructErrors(rootPath string, raw map[string]interface{}, err error, profile *ProfileService, report func(sev findings.Severity, code, path, msg, value string)) {
	work := cloneJSON(raw)
	for i := 0; err != nil && i < maxStructErrors; i++ {
		e := classifyStructError(err)
		rel := e.path
		if rel == "" && e.locate {
			rel = locateValue(work, e.value)
		}
		path := rootPath
		if rel != "" {
			shown := rel
			if profile != nil {
				shown = profile.ChoicePath(rootPath, rel)
			}
			path += "." + shown
		}
		value := e.value
		if value == "" && e.locate {
			value = "(empty)"
		}
		report(findings.Error, e.code, path, e.msg, value)
		if rel == "" || !removePath(work, strings.Split(rel, ".")) {
			return // cannot tell which field it was, so cannot look past it
		}
		err = unmarshalInto(rootPath, work)
	}
}

// unmarshalInto checks obj against the typed model of resourceType.
func unmarshalInto(resourceType string, obj map[string]interface{}) error {
	target, err := newFHIRResource(resourceType)
	if err != nil {
		return nil
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

func cloneJSON(m map[string]interface{}) map[string]interface{} {
	b, _ := json.Marshal(m)
	var out map[string]interface{}
	_ = json.Unmarshal(b, &out)
	return out
}

// locateValue returns the path (without array indexes, relative to obj) of the
// first leaf whose text is value, in key order; "" when there is none.
func locateValue(obj map[string]interface{}, value string) string {
	var found []string
	var walk func(prefix string, v interface{})
	walk = func(prefix string, v interface{}) {
		switch x := v.(type) {
		case map[string]interface{}:
			for k, c := range x {
				p := k
				if prefix != "" {
					p = prefix + "." + k
				}
				walk(p, c)
			}
		case []interface{}:
			for _, c := range x {
				walk(prefix, c)
			}
		case string:
			if x == value && prefix != "" {
				found = append(found, prefix)
			}
		case float64:
			if value != "" && fmt.Sprint(x) == value && prefix != "" {
				found = append(found, prefix)
			}
		}
	}
	walk("", obj)
	if len(found) == 0 {
		return ""
	}
	sort.Strings(found)
	return found[0]
}

// removePath deletes the field at path from obj, in every element when an
// array is on the way. It reports whether anything was removed.
func removePath(v interface{}, path []string) bool {
	switch x := v.(type) {
	case []interface{}:
		removed := false
		for _, c := range x {
			if removePath(c, path) {
				removed = true
			}
		}
		return removed
	case map[string]interface{}:
		if len(path) == 1 {
			_, ok := x[path[0]]
			delete(x, path[0])
			return ok
		}
		return removePath(x[path[0]], path[1:])
	}
	return false
}

var (
	// enumErrRe matches the error of a generated code type, e.g.
	// "unknown AdministrativeGender code `man`".
	enumErrRe   = regexp.MustCompile("^(.*) `(.*)`$")
	numberErrRe = regexp.MustCompile(`invalid number literal, trying to unmarshal "\\"(.*)\\"" into Number`)
	dateErrRe   = regexp.MustCompile(`^failed to parse date (.*?): `)
	dtErrRe     = regexp.MustCompile(`^invalid datetime format: (.*?) \(last error`)
)

// structError is one classified error of the typed model.
type structError struct {
	code   string
	path   string // relative to the resource; "" when the error does not say
	msg    string // the same for every resource with this kind of problem
	value  string // the offending value, if the error carries one
	locate bool   // path is unknown but can be found by searching for value
}

// classifyStructError turns an error from validateThroughStruct into a rule
// code, a message and, when the error says so, the field and the value.
func classifyStructError(err error) structError {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return structError{code: findings.CodeDataType, path: typeErr.Field,
			msg: fmt.Sprintf("expected %s, got JSON %s", typeErr.Type, typeErr.Value)}
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return structError{code: findings.CodeDataValue, msg: "malformed JSON"}
	}
	// Strip the "<Type> validation failed:" wrapper validateThroughStruct adds.
	text := err.Error()
	if i := strings.Index(text, "validation failed: "); i != -1 {
		text = text[i+len("validation failed: "):]
	}
	if m := enumErrRe.FindStringSubmatch(text); m != nil {
		return structError{code: findings.CodeDataValue, msg: m[1], value: m[2], locate: true}
	}
	if m := numberErrRe.FindStringSubmatch(text); m != nil {
		return structError{code: findings.CodeDataType, msg: "not a valid number", value: m[1], locate: true}
	}
	if m := dateErrRe.FindStringSubmatch(text); m != nil {
		return structError{code: findings.CodeDataValue, msg: "invalid date, expected YYYY-MM-DD", value: m[1], locate: true}
	}
	if m := dtErrRe.FindStringSubmatch(text); m != nil {
		return structError{code: findings.CodeDataValue, msg: "invalid dateTime, expected ISO 8601", value: m[1], locate: true}
	}
	return structError{code: findings.CodeDataValue, msg: text}
}

// checkProfile compares a resource with the element rules of the loaded
// profile for its type. It walks from the root through the elements that are
// present, so a required child of an absent optional parent is not reported.
func (fc *FHIRConverter) checkProfile(rootPath string, raw map[string]interface{}, report func(sev findings.Severity, code, path, msg, value string)) {
	var walk func(path string, obj map[string]interface{})
	walk = func(path string, obj map[string]interface{}) {
		for _, rule := range fc.profile.ChildElements(path) {
			name := rule.Path[len(path)+1:]
			isChoice := strings.HasSuffix(name, "[x]")

			var present []string // keys of obj that hold this element
			if isChoice {
				base := strings.TrimSuffix(name, "[x]")
				for k := range obj {
					if strings.HasPrefix(k, base) && len(k) > len(base) && k[len(base)] >= 'A' && k[len(base)] <= 'Z' {
						present = append(present, k)
					}
				}
			} else if v, ok := obj[name]; ok && !isEmptyValue(v) {
				present = []string{name}
			}

			switch {
			case rule.Prohibited() && len(present) > 0:
				report(findings.Error, findings.CodeDataProhibited, rule.Path, "element is prohibited by the profile (max 0)", "")
				continue
			case rule.Min >= 1 && len(present) == 0:
				report(findings.Error, findings.CodeDataRequired, rule.Path, fmt.Sprintf("required element is missing (min %d)", rule.Min), "")
				continue
			}

			if isChoice {
				base := strings.TrimSuffix(name, "[x]")
				for _, k := range present {
					if len(rule.Types) > 0 && !choiceTypeAllowed(k[len(base):], rule.Types) {
						report(findings.Error, findings.CodeDataChoiceType, rule.Path,
							fmt.Sprintf("type not allowed (allowed: %s)", strings.Join(rule.Types, ", ")), k[len(base):])
					}
				}
				for _, k := range present {
					if s, ok := obj[k].(string); ok {
						if msg := formatProblem(strings.ToLower(k[len(base):][:1])+k[len(base):][1:], s); msg != "" {
							report(findings.Error, findings.CodeDataValue, rule.Path, msg, s)
						}
					}
				}
				continue
			}
			if len(present) == 0 {
				continue
			}

			val := obj[name]
			if s, ok := val.(string); ok && len(rule.Types) == 1 {
				if msg := formatProblem(rule.Types[0], s); msg != "" {
					report(findings.Error, findings.CodeDataValue, rule.Path, msg, s)
				}
			}
			if list, ok := val.([]interface{}); ok && rule.Max == "1" && len(list) > 1 {
				report(findings.Warning, findings.CodeDataCardinal, rule.Path, "more than one value for an element with max 1", fmt.Sprint(len(list)))
			}
			if s, ok := val.(string); ok && rule.Fixed != "" && s != rule.Fixed {
				report(findings.Error, findings.CodeDataFixed, rule.Path, fmt.Sprintf("value must be %q", rule.Fixed), s)
			}

			switch v := val.(type) {
			case map[string]interface{}:
				walk(rule.Path, v)
			case []interface{}:
				for _, item := range v {
					if m, ok := item.(map[string]interface{}); ok {
						walk(rule.Path, m)
					}
				}
			}
		}
	}
	walk(rootPath, raw)
}

// choiceTypeAllowed reports whether the suffix of a choice element's key
// ("DateTime" in effectiveDateTime) is one of the profile's type codes
// ("dateTime"). FHIR primitive type codes start lowercase, the suffix does not.
func choiceTypeAllowed(suffix string, types []string) bool {
	for _, t := range types {
		if strings.EqualFold(suffix, t) {
			return true
		}
	}
	return false
}

func isEmptyValue(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []interface{}:
		return len(x) == 0
	case map[string]interface{}:
		return len(x) == 0
	}
	return false
}

var dateTimeRe = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2}(T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2}))?)?)?$`)

// formatProblem checks the date types the typed model keeps as plain strings
// (Patient.deceasedDateTime, Observation.issued, ...). typeCode is the FHIR
// type ("date", "dateTime", "instant"); other types are not checked. It returns
// a message that is the same for every bad value, or "".
func formatProblem(typeCode, s string) string {
	switch typeCode {
	case "date":
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return "invalid date, expected YYYY-MM-DD"
		}
	case "dateTime", "instant":
		ok := dateTimeRe.MatchString(s)
		if ok && len(s) >= 10 {
			_, err := time.Parse("2006-01-02", s[:10])
			ok = err == nil
		}
		if !ok {
			return "invalid dateTime, expected ISO 8601"
		}
	}
	return ""
}

// Profile returns the profile service the converter validates against, so tools such as the
// findings dashboard can look up an element's binding.
func (fc *FHIRConverter) Profile() *ProfileService { return fc.profile }

// SetObservationDefinitions makes the converter check every Observation's value against the
// ObservationDefinition of its code. Call it before the converter is shared between goroutines.
func (fc *FHIRConverter) SetObservationDefinitions(o *ObservationDefinitions) { fc.obsDefs = o }

// ObservationDefinitions returns the definitions the converter checks against, or nil.
func (fc *FHIRConverter) ObservationDefinitions() *ObservationDefinitions { return fc.obsDefs }

// elementPath names an element below the resource the way the profile does (choice types as
// "value[x]"), so findings from different checks land on the same element.
func (fc *FHIRConverter) elementPath(root, rel string) string {
	if fc.profile != nil {
		return fc.profile.ChoicePath(root, rel)
	}
	return rel
}

// observationCodes returns the codings of Observation.code.
func observationCodes(raw map[string]interface{}) []codeRef {
	cc, _ := raw["code"].(map[string]interface{})
	return codingsOf(cc)
}

// observationValue finds the Observation's value[x]: its key ("valueQuantity"), its type
// ("Quantity") and, for a CodeableConcept, the codes of its codings.
func observationValue(raw map[string]interface{}) (key, valueType string, answers []string) {
	for k, v := range raw {
		if len(k) > 5 && strings.HasPrefix(k, "value") && k[5] >= 'A' && k[5] <= 'Z' && !isEmptyValue(v) {
			key, valueType = k, k[5:]
			if m, ok := v.(map[string]interface{}); ok && valueType == "CodeableConcept" {
				for _, c := range codingsOf(m) {
					answers = append(answers, c.Code)
				}
			}
			return key, valueType, answers
		}
	}
	return "", "", nil
}

// codingsOf reads the coding (an array, or a single object that is not yet wrapped) of a CodeableConcept.
func codingsOf(cc map[string]interface{}) []codeRef {
	var items []interface{}
	switch c := cc["coding"].(type) {
	case []interface{}:
		items = c
	case map[string]interface{}:
		items = []interface{}{c}
	}
	var out []codeRef
	for _, it := range items {
		if m, ok := it.(map[string]interface{}); ok {
			code, _ := m["code"].(string)
			system, _ := m["system"].(string)
			if code != "" {
				out = append(out, codeRef{system, code})
			}
		}
	}
	return out
}

// mapObservationAnswers translates the answer of a coded Observation through the concept map that
// targets the answer valueset of its ObservationDefinition, so a source answer such as "P1" can be
// mapped onto the valid answer "L1" for one observation code. Answers with no mapping are left as
// they are; the check then reports them (FX-DATA-031).
func (fc *FHIRConverter) mapObservationAnswers(rootPath string, raw map[string]interface{}) {
	if fc.obsDefs == nil || fc.conceptMap == nil || rootPath != "Observation" {
		return
	}
	value, ok := raw["valueCodeableConcept"].(map[string]interface{})
	if !ok {
		return
	}
	for _, oc := range observationCodes(raw) {
		if _, url, _, ok := fc.obsDefs.AnswerSet(oc.Code); ok {
			translateCodingLike(value, url, fc.conceptMap, "Observation.valueCodeableConcept", nil)
			return
		}
	}
}
