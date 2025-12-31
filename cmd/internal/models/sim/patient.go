package sim

import (
	"time"
)

type Patient struct {
	Identificatienummer        *string    `csv:"Identificatienummer,omitempty" json:"identificatienummer,omitempty" parquet:"Identificatienummer"`
	GerelateerdPersoonID       *string    `csv:"GerelateerdPersoonID,omitempty" json:"gerelateerdPersoonID,omitempty" parquet:"GerelateerdPersoonID"`
	GerelateerdeRelatieSysteem *string    `csv:"GerelateerdeRelatieSysteem,omitempty" json:"gerelateerderelatiesysteem,omitempty" parquet:"GerelateerdeRelatieSysteem"`
	GerelateerdeRelatie        *string    `csv:"GerelateerdeRelatie,omitempty" json:"gerelateerdeRelatie,omitempty" parquet:"GerelateerdeRelatie"`
	GeslachtCodeSysteem        *string    `csv:"GeslachtCodeSysteem,omitempty" json:"geslachtcodesysteem,omitempty" parquet:"GeslachtCodeSysteem"`
	GeslachtCode               *string    `csv:"GeslachtCode,omitempty" json:"geslachtCode,omitempty" parquet:"GeslachtCode"`
	GeslachtOmschrijving       *string    `csv:"GeslachtOmschrijving,omitempty" json:"geslachtOmschrijving,omitempty" parquet:"GeslachtOmschrijving"`
	LandSysteem                *string    `csv:"LandSysteem,omitempty" json:"landsysteem,omitempty" parquet:"LandSysteem"`
	Land                       *string    `csv:"Land,omitempty" json:"land,omitempty" parquet:"Land"`
	Geboortedatum              *time.Time `csv:"Geboortedatum,omitempty" json:"geboortedatum,omitempty" parquet:"Geboortedatum"`
	DatumOverlijden            *time.Time `csv:"DatumOverlijden,omitempty" json:"datumOverlijden,omitempty" parquet:"DatumOverlijden"`
	DatumCheckStatusOverlijden *time.Time `csv:"DatumCheckStatusOverlijden,omitempty" json:"datumCheckStatusOverlijden,omitempty" parquet:"DatumCheckStatusOverlijden"`
}
