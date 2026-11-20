package fhirserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SanteonNL/fenix/internal/conformance"
	"github.com/google/uuid"
)

// SetConformance makes the server serve the conformance resources of store
// (search by url, read, ValueSet/$expand, ValueSet/$validate-code) and use its
// ValueSets for the :in / :not-in search modifiers. Call before serving.
func (s *Server) SetConformance(store *conformance.Store) { s.conformance = store }

// valueSetMembers is the expander for :in / :not-in, or nil without a store.
func (s *Server) valueSetMembers() valueSetMembersFunc {
	if s.conformance == nil {
		return nil
	}
	return s.conformance.Members
}

// filterDeps are the expander for :in / :not-in and the resolver for chained params.
func (s *Server) filterDeps() filterDeps {
	return filterDeps{expand: s.valueSetMembers(), chain: s.chainIDs}
}

// chainIDs resolves a chained param: the ids of the targetType resources that
// match targetParam=value, found with an ordinary search on targetType.
func (s *Server) chainIDs(targetType, targetParam, value string) (map[string]bool, error) {
	resources, found, err := s.searchContext(context.Background(), targetType, map[string]string{targetParam: value})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("no queries configured for resource type %q in source %q", targetType, s.source)
	}
	ids := make(map[string]bool, len(resources))
	for _, r := range resources {
		b, err := json.Marshal(r)
		if err != nil {
			continue
		}
		var head struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(b, &head) == nil && head.ID != "" {
			ids[head.ID] = true
		}
	}
	return ids, nil
}

// checkSearchParams refuses a search FENIX cannot evaluate correctly, instead
// of silently filtering everything (or nothing) away: an :in / :not-in
// ValueSet that cannot be expanded, or a chained param whose target type has
// no queries.
func (s *Server) checkSearchParams(fhirParams map[string]string) error {
	if err := s.checkValueSetParams(fhirParams); err != nil {
		return err
	}
	for code := range fhirParams {
		if _, target, targetParam, ok := chainedParam(code); ok {
			rendered, err := s.render(target, map[string]string{targetParam: fhirParams[code]}, "")
			if err != nil {
				return fmt.Errorf("%s: %w", code, err)
			}
			if len(rendered) == 0 {
				return fmt.Errorf("%s: resource type %q is not configured for source %q", code, target, s.source)
			}
		}
	}
	return nil
}

// checkValueSetParams refuses a search whose :in / :not-in ValueSets cannot be
// expanded.
func (s *Server) checkValueSetParams(fhirParams map[string]string) error {
	for code, urls := range valueSetParams(fhirParams) {
		if s.conformance == nil {
			return fmt.Errorf("%s: no ValueSets are loaded (configure inputs.valueSets, inputs.packages or inputs.terminologyServers)", code)
		}
		for _, u := range urls {
			if _, err := s.conformance.Members(unescape(u)); err != nil {
				return fmt.Errorf("%s: %w", code, err)
			}
		}
	}
	return nil
}

// handleConformance serves /r4/<type>[/<id>][/$operation] for the conformance resource types.
func (s *Server) handleConformance(w http.ResponseWriter, r *http.Request, resourceType string, rest []string) {
	if s.conformance == nil {
		fhirError(w, fmt.Sprintf("no %s resources are loaded (configure inputs.packages or the terminology folders)", resourceType), http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet && !(r.Method == http.MethodPost && len(rest) > 0 && strings.HasPrefix(rest[len(rest)-1], "$")) {
		fhirError(w, "method not allowed; conformance resources are read-only", http.StatusMethodNotAllowed)
		return
	}
	params, err := operationParams(r)
	if err != nil {
		fhirError(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch {
	case len(rest) == 0 || (len(rest) == 1 && rest[0] == ""):
		s.conformanceSearch(w, r, resourceType, params)
	case len(rest) == 1 && strings.HasPrefix(rest[0], "$"):
		s.conformanceOperation(w, resourceType, rest[0], "", params)
	case len(rest) == 1:
		e, ok := s.conformance.Read(resourceType, rest[0])
		if !ok {
			fhirError(w, fmt.Sprintf("%s/%s not found", resourceType, rest[0]), http.StatusNotFound)
			return
		}
		writeFHIR(w, http.StatusOK, e.Raw)
	case len(rest) == 2 && strings.HasPrefix(rest[1], "$"):
		s.conformanceOperation(w, resourceType, rest[1], rest[0], params)
	default:
		fhirError(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) conformanceSearch(w http.ResponseWriter, r *http.Request, resourceType string, params map[string]string) {
	count, offset, err := parsePaging(params)
	if err != nil {
		fhirError(w, err.Error(), http.StatusBadRequest)
		return
	}
	delete(params, "_count")
	delete(params, "_offset")
	delete(params, "_format")
	found, err := s.conformance.Search(resourceType, params)
	if err != nil {
		fhirError(w, err.Error(), http.StatusBadRequest)
		return
	}
	total := len(found)
	if count > 0 {
		if offset > len(found) {
			offset = len(found)
		}
		end := offset + count
		if end > len(found) {
			end = len(found)
		}
		found = found[offset:end]
	}
	entries := make([]map[string]any, len(found))
	for i, e := range found {
		entries[i] = map[string]any{"fullUrl": resourceType + "/" + e.ID, "resource": e.Raw}
	}
	bundle := map[string]any{"resourceType": "Bundle", "type": "searchset", "total": total, "entry": entries}
	if count > 0 {
		bundle["link"] = pagingLinks(r.URL, offset, count, total)
	}
	writeFHIRJSON(w, http.StatusOK, bundle)
}

func (s *Server) conformanceOperation(w http.ResponseWriter, resourceType, op, id string, params map[string]string) {
	if resourceType != "ValueSet" {
		fhirError(w, fmt.Sprintf("operation %s is not supported on %s", op, resourceType), http.StatusNotFound)
		return
	}
	ref := params["url"]
	if v := params["valueSetVersion"]; v != "" && ref != "" {
		ref += "|" + v
	}
	if id != "" {
		ref = "ValueSet/" + id
	}
	if ref == "" {
		fhirError(w, op+" needs a url parameter or a ValueSet id in the path", http.StatusBadRequest)
		return
	}
	switch op {
	case "$expand":
		s.valueSetExpand(w, ref, params)
	case "$validate-code":
		s.valueSetValidateCode(w, ref, params)
	default:
		fhirError(w, fmt.Sprintf("operation %s is not supported (supported: $expand, $validate-code)", op), http.StatusNotFound)
	}
}

func (s *Server) valueSetExpand(w http.ResponseWriter, ref string, params map[string]string) {
	x, err := s.conformance.Expand(ref)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if strings.Contains(err.Error(), "is not loaded") {
			status = http.StatusNotFound
		}
		fhirError(w, err.Error(), status)
		return
	}
	concepts := x.Concepts
	if f := strings.ToLower(params["filter"]); f != "" {
		var keep []conformance.Concept
		for _, c := range concepts {
			if strings.Contains(strings.ToLower(c.Display), f) || strings.Contains(strings.ToLower(c.Code), f) {
				keep = append(keep, c)
			}
		}
		concepts = keep
	}
	total := len(concepts)
	offset, _ := strconv.Atoi(params["offset"])
	if offset < 0 || offset > total {
		offset = total
	}
	page := concepts[offset:]
	if n, err := strconv.Atoi(params["count"]); err == nil && n >= 0 && n < len(page) {
		page = page[:n]
	}

	vs := map[string]any{}
	if x.Entry != nil {
		_ = json.Unmarshal(x.Entry.Raw, &vs)
	} else {
		vs["resourceType"], vs["url"], vs["status"] = "ValueSet", x.URL, "active"
		if x.Version != "" {
			vs["version"] = x.Version
		}
	}
	contains := make([]conformance.Concept, len(page))
	copy(contains, page)
	vs["expansion"] = map[string]any{
		"identifier": "urn:uuid:" + uuid.NewString(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"total":      total,
		"offset":     offset,
		"parameter":  []map[string]any{{"name": "expansion-source", "valueString": x.Source}},
		"contains":   contains,
	}
	writeFHIRJSON(w, http.StatusOK, vs)
}

func (s *Server) valueSetValidateCode(w http.ResponseWriter, ref string, params map[string]string) {
	code, system := params["code"], params["system"]
	if code == "" {
		fhirError(w, "$validate-code needs a code parameter", http.StatusBadRequest)
		return
	}
	found, display, err := s.conformance.ValidateCode(ref, system, code)
	if err != nil {
		fhirError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	out := []map[string]any{{"name": "result", "valueBoolean": found}}
	if found && display != "" {
		out = append(out, map[string]any{"name": "display", "valueString": display})
		if want := params["display"]; want != "" && !strings.EqualFold(want, display) {
			out = append(out, map[string]any{"name": "message", "valueString": fmt.Sprintf("display %q does not match %q", want, display)})
		}
	}
	if !found {
		out = append(out, map[string]any{"name": "message", "valueString": fmt.Sprintf("%s|%s is not in ValueSet %s", system, code, ref)})
	}
	writeFHIRJSON(w, http.StatusOK, map[string]any{"resourceType": "Parameters", "parameter": out})
}

// operationParams collects the parameters of a request: the query string, and
// for a POST a Parameters resource (valueUri / valueString / valueCode /
// valueCoding) or a form body.
func operationParams(r *http.Request) (map[string]string, error) {
	params := map[string]string{}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	if r.Method != http.MethodPost {
		return params, nil
	}
	if strings.Contains(r.Header.Get("Content-Type"), "json") {
		var p struct {
			ResourceType string `json:"resourceType"`
			Parameter    []struct {
				Name        string `json:"name"`
				ValueURI    string `json:"valueUri"`
				ValueString string `json:"valueString"`
				ValueCode   string `json:"valueCode"`
				ValueCanon  string `json:"valueCanonical"`
				ValueCoding *struct {
					System, Code, Display string
				} `json:"valueCoding"`
			} `json:"parameter"`
		}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.ResourceType != "Parameters" {
			return nil, fmt.Errorf("POST body must be a FHIR Parameters resource")
		}
		for _, q := range p.Parameter {
			switch {
			case q.ValueCoding != nil:
				params["system"], params["code"] = q.ValueCoding.System, q.ValueCoding.Code
				if q.ValueCoding.Display != "" {
					params["display"] = q.ValueCoding.Display
				}
			default:
				params[q.Name] = q.ValueURI + q.ValueString + q.ValueCode + q.ValueCanon
			}
		}
		return params, nil
	}
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	for k, v := range r.PostForm {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	return params, nil
}

func writeFHIR(w http.ResponseWriter, status int, raw []byte) {
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func writeFHIRJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
