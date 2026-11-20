package fhirserver

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/SanteonNL/fenix/internal/datasets"
	"github.com/SanteonNL/fenix/internal/findings"
)

// SetDatasets gives the server the dataset definitions of the loaded packages
// (package use "datasets"). A Group posted with the id the IG gives a dataset's
// cohort (<dataset>-cohort) is linked to that dataset, and its $export reports
// a finding for every element the dataset's data requirements need that a
// resource lacks. Call before serving; nil checks nothing.
func (s *Server) SetDatasets(c *datasets.Catalog) { s.datasets = c }

// datasetForGroup returns the dataset a posted Group was linked to when it was created.
func (s *Server) datasetForGroup(groupID string) *datasets.Dataset {
	s.groupsMu.RLock()
	id := s.groupDatasets[groupID]
	s.groupsMu.RUnlock()
	if id == "" {
		return nil
	}
	ds, _ := s.datasets.Get(id)
	return ds
}

// datasetFindingPath is the element path of a dataset finding, "Procedure.performed[x]".
func datasetFindingPath(resourceType, element string) string { return resourceType + "." + element }

func datasetMessage(ds *datasets.Dataset, by []string, what string) string {
	return fmt.Sprintf("%s by dataset %s (%s)", what, ds.ID, strings.Join(by, ", "))
}

// reportMissingDatasetTypes reports every resource type the dataset needs that
// the export does not ask for in _type: none of its required elements will be delivered.
func (s *Server) reportMissingDatasetTypes(ds *datasets.Dataset, resourceTypes []string) {
	c := s.converter.Findings()
	if c == nil || ds == nil {
		return
	}
	asked := make(map[string]bool, len(resourceTypes))
	for _, t := range resourceTypes {
		asked[t] = true
	}
	for _, t := range ds.Types() {
		if asked[t] {
			continue
		}
		var by []string
		for _, b := range ds.Elements[t] {
			for _, x := range b {
				if !containsString(by, x) {
					by = append(by, x)
				}
			}
		}
		sort.Strings(by)
		c.Add(findings.Warning, findings.CodeDataDatasetType, t, "", t,
			datasetMessage(ds, by, "resource type is needed but not in the export's _type"), "", "")
	}
}

// checkDatasetElements reports, for every resource of one exported type, each
// element the dataset's data requirements need that the resource does not
// hold, and takes such a resource out of the converter's clean count. It runs
// on the resources as the converter built them, before the active patient
// number, _elements trimming and de-identification change them, so the ids
// are the ones the converter reported.
func (s *Server) checkDatasetElements(ds *datasets.Dataset, resourceType string, resources []interface{}) {
	if ds == nil {
		return
	}
	required := ds.Elements[resourceType]
	c := s.converter.Findings()
	if len(required) == 0 || c == nil {
		return
	}
	elements := make([]string, 0, len(required))
	for e := range required {
		elements = append(elements, e)
	}
	sort.Strings(elements)

	for _, r := range resources {
		b, err := json.Marshal(r)
		if err != nil {
			continue
		}
		var fields map[string]interface{}
		if json.Unmarshal(b, &fields) != nil {
			continue
		}
		id, _ := fields["id"].(string)
		for _, e := range elements {
			if datasets.Present(fields, e) {
				continue
			}
			c.NotClean(resourceType, id) // the converter counted it clean before this check ran
			path := datasetFindingPath(resourceType, e)
			msg := datasetMessage(ds, required[e], "element is needed")
			var sample []byte
			if c.NeedsSample(findings.CodeDataDatasetElement, resourceType, "", path, msg) {
				sample = b
			}
			c.AddSample(findings.Warning, findings.CodeDataDatasetElement, resourceType, "", path, msg, id, "", sample)
		}
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
