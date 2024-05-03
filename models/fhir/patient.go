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

import "encoding/json"

// THIS FILE IS GENERATED BY https://github.com/samply/golang-fhir-models
// PLEASE DO NOT EDIT BY HAND

// Patient is documented here http://hl7.org/fhir/StructureDefinition/Patient
type Patient struct {
	Id                   *string                `bson:"id,omitempty" json:"id,omitempty" db:"id"`
	Meta                 *Meta                  `bson:"meta,omitempty" json:"meta,omitempty" db:"meta"`
	ImplicitRules        *string                `bson:"implicitRules,omitempty" json:"implicitRules,omitempty" db:"implicit_rules"`
	Language             *string                `bson:"language,omitempty" json:"language,omitempty" db:"language"`
	Text                 *Narrative             `bson:"text,omitempty" json:"text,omitempty" db:"text"`
	Extension            []Extension            `bson:"extension,omitempty" json:"extension,omitempty" db:"extension"`
	ModifierExtension    []Extension            `bson:"modifierExtension,omitempty" json:"modifierExtension,omitempty" db:"modifier_extension"`
	Identifier           []Identifier           `bson:"identifier,omitempty" json:"identifier,omitempty" db:"identifier"`
	Active               *bool                  `bson:"active,omitempty" json:"active,omitempty" db:"active"`
	Name                 []HumanName            `bson:"name,omitempty" json:"name,omitempty" db:"name"`
	Telecom              []ContactPoint         `bson:"telecom,omitempty" json:"telecom,omitempty" db:"telecom"`
	Gender               *AdministrativeGender  `bson:"gender,omitempty" json:"gender,omitempty" db:"gender"`
	BirthDate            *string                `bson:"birthDate,omitempty" json:"birthDate,omitempty" db:"birth_date"`
	DeceasedBoolean      *bool                  `bson:"deceasedBoolean,omitempty" json:"deceasedBoolean,omitempty" db:"deceased_boolean"`
	DeceasedDateTime     *string                `bson:"deceasedDateTime,omitempty" json:"deceasedDateTime,omitempty" db:"deceased_date_time"`
	Address              []Address              `bson:"address,omitempty" json:"address,omitempty" db:"address"`
	MaritalStatus        *CodeableConcept       `bson:"maritalStatus,omitempty" json:"maritalStatus,omitempty" db:"marital_status"`
	MultipleBirthBoolean *bool                  `bson:"multipleBirthBoolean,omitempty" json:"multipleBirthBoolean,omitempty" db:"multiple_birth_boolean"`
	MultipleBirthInteger *int                   `bson:"multipleBirthInteger,omitempty" json:"multipleBirthInteger,omitempty" db:"multiple_birth_integer"`
	Photo                []Attachment           `bson:"photo,omitempty" json:"photo,omitempty" db:"photo"`
	Contact              []PatientContact       `bson:"contact,omitempty" json:"contact,omitempty" db:"contact"`
	Communication        []PatientCommunication `bson:"communication,omitempty" json:"communication,omitempty" db:"communication"`
	GeneralPractitioner  []Reference            `bson:"generalPractitioner,omitempty" json:"generalPractitioner,omitempty" db:"general_practitioner"`
	ManagingOrganization *Reference             `bson:"managingOrganization,omitempty" json:"managingOrganization,omitempty" db:"managing_organization"`
	Link                 []PatientLink          `bson:"link,omitempty" json:"link,omitempty" db:"link"`
}

type PatientContact struct {
	Id                *string               `bson:"id,omitempty" json:"id,omitempty"`
	Extension         []Extension           `bson:"extension,omitempty" json:"extension,omitempty"`
	ModifierExtension []Extension           `bson:"modifierExtension,omitempty" json:"modifierExtension,omitempty"`
	Relationship      []CodeableConcept     `bson:"relationship,omitempty" json:"relationship,omitempty"`
	Name              *HumanName            `bson:"name,omitempty" json:"name,omitempty"`
	Telecom           []ContactPoint        `bson:"telecom,omitempty" json:"telecom,omitempty"`
	Address           *Address              `bson:"address,omitempty" json:"address,omitempty"`
	Gender            *AdministrativeGender `bson:"gender,omitempty" json:"gender,omitempty"`
	Organization      *Reference            `bson:"organization,omitempty" json:"organization,omitempty"`
	Period            *Period               `bson:"period,omitempty" json:"period,omitempty"`
}
type PatientCommunication struct {
	Id                *string         `bson:"id,omitempty" json:"id,omitempty"`
	Extension         []Extension     `bson:"extension,omitempty" json:"extension,omitempty"`
	ModifierExtension []Extension     `bson:"modifierExtension,omitempty" json:"modifierExtension,omitempty"`
	Language          CodeableConcept `bson:"language" json:"language"`
	Preferred         *bool           `bson:"preferred,omitempty" json:"preferred,omitempty"`
}
type PatientLink struct {
	Id                *string     `bson:"id,omitempty" json:"id,omitempty"`
	Extension         []Extension `bson:"extension,omitempty" json:"extension,omitempty"`
	ModifierExtension []Extension `bson:"modifierExtension,omitempty" json:"modifierExtension,omitempty"`
	Other             Reference   `bson:"other" json:"other"`
	Type              LinkType    `bson:"type" json:"type"`
}
type OtherPatient Patient

// MarshalJSON marshals the given Patient as JSON into a byte slice
func (r Patient) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		OtherPatient
		ResourceType string `json:"resourceType"`
	}{
		OtherPatient: OtherPatient(r),
		ResourceType: "Patient",
	})
}

// UnmarshalPatient unmarshals a Patient.
func UnmarshalPatient(b []byte) (Patient, error) {
	var patient Patient
	if err := json.Unmarshal(b, &patient); err != nil {
		return patient, err
	}
	return patient, nil
}