// Copyright 2019 - 2022 The Samply Community
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fhir

import (
	"encoding/json"
	"fmt"
	"strings"
)

// THIS FILE IS GENERATED BY https://github.com/samply/golang-fhir-models
// PLEASE DO NOT EDIT BY HAND

// FilterOperator is documented here http://hl7.org/fhir/ValueSet/filter-operator
type FilterOperator int

const (
	FilterOperatorEquals FilterOperator = iota
	FilterOperatorIsA
	FilterOperatorDescendentOf
	FilterOperatorIsNotA
	FilterOperatorRegex
	FilterOperatorIn
	FilterOperatorNotIn
	FilterOperatorGeneralizes
	FilterOperatorExists
)

func (code FilterOperator) MarshalJSON() ([]byte, error) {
	return json.Marshal(code.Code())
}
func (code *FilterOperator) UnmarshalJSON(json []byte) error {
	s := strings.Trim(string(json), "\"")
	switch s {
	case "=":
		*code = FilterOperatorEquals
	case "is-a":
		*code = FilterOperatorIsA
	case "descendent-of":
		*code = FilterOperatorDescendentOf
	case "is-not-a":
		*code = FilterOperatorIsNotA
	case "regex":
		*code = FilterOperatorRegex
	case "in":
		*code = FilterOperatorIn
	case "not-in":
		*code = FilterOperatorNotIn
	case "generalizes":
		*code = FilterOperatorGeneralizes
	case "exists":
		*code = FilterOperatorExists
	default:
		return fmt.Errorf("unknown FilterOperator code `%s`", s)
	}
	return nil
}
func (code FilterOperator) String() string {
	return code.Code()
}
func (code FilterOperator) Code() string {
	switch code {
	case FilterOperatorEquals:
		return "="
	case FilterOperatorIsA:
		return "is-a"
	case FilterOperatorDescendentOf:
		return "descendent-of"
	case FilterOperatorIsNotA:
		return "is-not-a"
	case FilterOperatorRegex:
		return "regex"
	case FilterOperatorIn:
		return "in"
	case FilterOperatorNotIn:
		return "not-in"
	case FilterOperatorGeneralizes:
		return "generalizes"
	case FilterOperatorExists:
		return "exists"
	}
	return "<unknown>"
}
func (code FilterOperator) Display() string {
	switch code {
	case FilterOperatorEquals:
		return "Equals"
	case FilterOperatorIsA:
		return "Is A (by subsumption)"
	case FilterOperatorDescendentOf:
		return "Descendent Of (by subsumption)"
	case FilterOperatorIsNotA:
		return "Not (Is A) (by subsumption)"
	case FilterOperatorRegex:
		return "Regular Expression"
	case FilterOperatorIn:
		return "In Set"
	case FilterOperatorNotIn:
		return "Not in Set"
	case FilterOperatorGeneralizes:
		return "Generalizes (by Subsumption)"
	case FilterOperatorExists:
		return "Exists"
	}
	return "<unknown>"
}
func (code FilterOperator) Definition() string {
	switch code {
	case FilterOperatorEquals:
		return "The specified property of the code equals the provided value."
	case FilterOperatorIsA:
		return "Includes all concept ids that have a transitive is-a relationship with the concept Id provided as the value, including the provided concept itself (include descendant codes and self)."
	case FilterOperatorDescendentOf:
		return "Includes all concept ids that have a transitive is-a relationship with the concept Id provided as the value, excluding the provided concept itself i.e. include descendant codes only)."
	case FilterOperatorIsNotA:
		return "The specified property of the code does not have an is-a relationship with the provided value."
	case FilterOperatorRegex:
		return "The specified property of the code  matches the regex specified in the provided value."
	case FilterOperatorIn:
		return "The specified property of the code is in the set of codes or concepts specified in the provided value (comma separated list)."
	case FilterOperatorNotIn:
		return "The specified property of the code is not in the set of codes or concepts specified in the provided value (comma separated list)."
	case FilterOperatorGeneralizes:
		return "Includes all concept ids that have a transitive is-a relationship from the concept Id provided as the value, including the provided concept itself (i.e. include ancestor codes and self)."
	case FilterOperatorExists:
		return "The specified property of the code has at least one value (if the specified value is true; if the specified value is false, then matches when the specified property of the code has no values)."
	}
	return "<unknown>"
}
