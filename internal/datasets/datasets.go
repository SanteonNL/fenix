// Package datasets reads the dataset definitions of the sim-on-fhir IG from a
// FHIR package and says which elements each dataset needs per resource type.
//
// The IG ships its logical-model instances as Binary resources whose data is
// the instance JSON (base64), e.g. Binary-sb-ibd.json holding
//
//	{"resourceType": ".../StructureDefinition/dataset-definition", "id": "sb-ibd",
//	 "include": [{"resource": "Patient", "element": ["gender", "birthDate"]}],
//	 "requirement": ["k312-scopie", ...]}
//
// and Binary-k312-scopie.json holding a data-requirement-definition with
// criteria[].resource and criteria[].element. A dataset needs the elements of
// its own include entries and of every criterion of its requirements.
package datasets

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// Logical-model type names, the last segment of the instance's resourceType URL.
const (
	datasetModel     = "dataset-definition"
	requirementModel = "data-requirement-definition"
)

// GroupSuffix is what the IG appends to a dataset id to name its cohort Group
// (sb-ibd -> Group/sb-ibd-cohort).
const GroupSuffix = "-cohort"

type criterion struct {
	Resource string   `json:"resource"`
	Element  []string `json:"element"`
	Param    []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"param"`
}

type definition struct {
	ResourceType string      `json:"resourceType"`
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Version      string      `json:"version"`
	Include      []criterion `json:"include"`     // dataset
	Requirement  []string    `json:"requirement"` // dataset: ids of data requirements
	Criteria     []criterion `json:"criteria"`    // data requirement
}

// Dataset is one dataset definition with its data requirements resolved.
type Dataset struct {
	ID      string
	Title   string
	Version string

	// Elements maps resource type -> element ("code", "value[x]") -> who needs
	// it: requirement ids, or "dataset <id>" for the dataset's own include.
	Elements map[string]map[string][]string

	// Filters maps resource type -> search param the data requirements filter
	// on ("code", "code:in", "date") -> the requirement ids that do. The values
	// are the IG's placeholders ({od:...}, {vs:...}); the resolved ones are in
	// the export's _typeFilter. The dataset's own include counts as "dataset <id>".
	Filters map[string]map[string][]string

	// Missing lists requirement ids the dataset names but the package does not hold.
	Missing []string
}

// Types returns the resource types the dataset needs elements of, sorted.
func (d *Dataset) Types() []string {
	out := make([]string, 0, len(d.Elements))
	for t := range d.Elements {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// Catalog holds the datasets and data requirements read so far. It is safe for
// concurrent use; a nil *Catalog holds nothing.
type Catalog struct {
	mu           sync.RWMutex
	datasets     map[string]definition
	requirements map[string]definition
}

func NewCatalog() *Catalog {
	return &Catalog{datasets: map[string]definition{}, requirements: map[string]definition{}}
}

// AddFiles reads Binary resource files and keeps the dataset and data
// requirement definitions among them; other Binaries are skipped. It returns
// how many definitions were added.
func (c *Catalog) AddFiles(paths []string) (int, error) {
	n := 0
	var errs []string
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		ok, err := c.Add(raw)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		if ok {
			n++
		}
	}
	if len(errs) > 0 {
		return n, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return n, nil
}

// Add reads one Binary resource (or the bare definition JSON) and keeps it when
// it is a dataset or data requirement definition.
func (c *Catalog) Add(raw []byte) (bool, error) {
	var bin struct {
		ResourceType string `json:"resourceType"`
		ContentType  string `json:"contentType"`
		Data         string `json:"data"`
	}
	if err := json.Unmarshal(raw, &bin); err != nil {
		return false, err
	}
	body := raw
	if bin.ResourceType == "Binary" {
		if bin.Data == "" || !strings.Contains(bin.ContentType, "json") {
			return false, nil
		}
		b, err := base64.StdEncoding.DecodeString(bin.Data)
		if err != nil {
			return false, fmt.Errorf("Binary data: %w", err)
		}
		body = b
	}
	var def definition
	if err := json.Unmarshal(body, &def); err != nil {
		return false, nil // a JSON Binary that is not a definition
	}
	model := def.ResourceType[strings.LastIndex(def.ResourceType, "/")+1:]
	if def.ID == "" || (model != datasetModel && model != requirementModel) {
		return false, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if model == datasetModel {
		c.datasets[def.ID] = def
	} else {
		c.requirements[def.ID] = def
	}
	return true, nil
}

// IDs returns the ids of the datasets, sorted.
func (c *Catalog) IDs() []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.datasets))
	for id := range c.datasets {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Get returns a dataset with its requirements resolved.
func (c *Catalog) Get(id string) (*Dataset, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	def, ok := c.datasets[id]
	if !ok {
		return nil, false
	}
	ds := &Dataset{ID: def.ID, Title: def.Title, Version: def.Version, Elements: map[string]map[string][]string{}, Filters: map[string]map[string][]string{}}
	add := func(crit []criterion, by string) {
		for _, cr := range crit {
			if cr.Resource == "" {
				continue
			}
			m := ds.Elements[cr.Resource]
			if m == nil {
				m = map[string][]string{}
				ds.Elements[cr.Resource] = m
			}
			for _, e := range cr.Element {
				if e != "" && !contains(m[e], by) {
					m[e] = append(m[e], by)
				}
			}
			if len(cr.Param) > 0 && ds.Filters[cr.Resource] == nil {
				ds.Filters[cr.Resource] = map[string][]string{}
			}
			for _, p := range cr.Param {
				if p.Name != "" && !contains(ds.Filters[cr.Resource][p.Name], by) {
					ds.Filters[cr.Resource][p.Name] = append(ds.Filters[cr.Resource][p.Name], by)
				}
			}
		}
	}
	add(def.Include, "dataset "+def.ID)
	for _, rid := range def.Requirement {
		req, ok := c.requirements[rid]
		if !ok {
			ds.Missing = append(ds.Missing, rid)
			continue
		}
		add(req.Criteria, rid)
	}
	return ds, true
}

// ForGroup returns the dataset a cohort Group belongs to, from the Group's own
// id as the IG generates it (<dataset>-cohort), or the dataset id itself.
func (c *Catalog) ForGroup(groupID string) (*Dataset, bool) {
	if groupID == "" {
		return nil, false
	}
	if ds, ok := c.Get(strings.TrimSuffix(groupID, GroupSuffix)); ok {
		return ds, true
	}
	return nil, false
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
