# Synthetische FHIR-data voor de IBD-dataset

Gegenereerd door [generate_ibd_synthetic.py](generate_ibd_synthetic.py). Alle waarden zijn willekeurig. Elke resource en elk element linkt naar de FHIR R4-specificatie.

## Wat staat waar?

Alles staat in de map [output/](output/). Je hoeft maar twee dingen te begrijpen: **wie** er in de dataset zit en **wat** er van die mensen wordt opgehaald.

```text
output/
├── Group.json         WIE: het cohort (de regels waaraan een patiënt moet voldoen)
├── Parameters.json    WAT: het exportverzoek (welke soorten gegevens, welke filters, welke velden)
├── source/            DE BRON: alle gegevens zoals het ziekenhuis ze heeft (60 patiënten)
│   ├── Patient.ndjson
│   ├── EpisodeOfCare.ndjson    DBC's (zorgtrajecten)
│   ├── Condition.ndjson        de diagnose bij een DBC
│   ├── Procedure.ndjson        verrichtingen, zoals scopieën
│   └── Observation.ndjson      metingen: calprotectine en alcoholgebruik
└── export/            HET RESULTAAT: wat de uitvraag oplevert
    ├── Patient.ndjson
    ├── Procedure.ndjson
    └── Observation.ndjson
```

**Hoe lees je het?** Begin bij `Group.json` en `Parameters.json`: samen beschrijven ze de dataset. `source/` is wat er in het ziekenhuis staat, ook voor patiënten die niet in het cohort zitten. `export/` is wat er uit `source/` overblijft als je `Group.json` en `Parameters.json` toepast. Vergelijk je `source/` met `export/`, dan zie je precies wat de uitvraag selecteert en weglaat.

**Wat is een `.ndjson`-bestand?** Een tekstbestand met één FHIR-resource (in JSON) per regel. Je opent het in elke teksteditor, bijvoorbeeld Notepad of VS Code. Elke regel is één patiënt, één DBC, één verrichting of één meting.

**Hoe volg je één patiënt?** Zoek in `source/Patient.ndjson` een `id` (een lange code, bijvoorbeeld `392456de-…`). Zoek die code in de andere bestanden: in het veld `patient` of `subject` staat `Patient/<die code>`. Zo vind je alle DBC's, diagnoses, verrichtingen en metingen van die patiënt.

**Zit een patiënt in het cohort?** Dat bepaalt `Group.json`: 18 jaar of ouder, én minstens één actieve DBC met zorgtype 11 of 21, gestart vanaf 2018 en met een IBD-diagnose. Staat de patiënt in `export/Patient.ndjson`, dan hoort die bij het cohort.

**Opnieuw genereren** (bijvoorbeeld met meer patiënten): open een terminal in deze map en draai `py -3 generate_ibd_synthetic.py --patients 200`. De map `output/` wordt dan overschreven.

## [Patient](https://hl7.org/fhir/R4/patient.html)

| Element | Uitleg |
|---|---|
| [`id`](https://hl7.org/fhir/R4/resource-definitions.html#Resource.id) | Willekeurige UUID. |
| [`identifier.value`](https://hl7.org/fhir/R4/patient-definitions.html#Patient.identifier) | Patiëntnummer, bijvoorbeeld `P100000`. |
| [`birthDate`](https://hl7.org/fhir/R4/patient-definitions.html#Patient.birthDate) | Geboortedatum. Bij cohortleden 18 jaar of ouder; bij de minderjarige testgevallen jonger. |
| [`gender`](https://hl7.org/fhir/R4/patient-definitions.html#Patient.gender) | `male` of `female`. |

## [EpisodeOfCare](https://hl7.org/fhir/R4/episodeofcare.html)

Elke patiënt heeft 1 tot 5 DBC's: een IBD-traject (eerste DBC zorgtype `11`, 0 tot 2 vervolg-DBC's zorgtype `21`, zelfde diagnose) en 0 tot 2 DBC's met een diagnose die niets met IBD te maken heeft.

| Element | Uitleg |
|---|---|
| [`identifier.value`](https://hl7.org/fhir/R4/episodeofcare-definitions.html#EpisodeOfCare.identifier) | DBC-subtrajectnummer, bijvoorbeeld `DBC1`. |
| [`status`](https://hl7.org/fhir/R4/episodeofcare-definitions.html#EpisodeOfCare.status) | Afgesloten DBC's `finished`; alleen de laatste IBD-DBC is `active` (testgeval niet-actief: ook die `finished`). |
| [`type.coding`](https://hl7.org/fhir/R4/episodeofcare-definitions.html#EpisodeOfCare.type) | Zorgtype: `11` of `21` (testgeval: `13`), systeem `urn:oid:2.16.840.1.113883.2.4.3.27.15.2`. |
| [`period.start`](https://hl7.org/fhir/R4/episodeofcare-definitions.html#EpisodeOfCare.period) | Openingsdatum van de DBC, vanaf 2018-01-01 (testgeval: ervoor). Een vervolg-DBC start na de vorige. |
| [`period.end`](https://hl7.org/fhir/R4/episodeofcare-definitions.html#EpisodeOfCare.period) | Alleen gevuld bij `finished`; de dag vóór de start van de vervolg-DBC. |
| [`diagnosis.condition`](https://hl7.org/fhir/R4/episodeofcare-definitions.html#EpisodeOfCare.diagnosis.condition) | Verwijzing naar de Condition met de specialisme-diagnose. |
| [`patient`](https://hl7.org/fhir/R4/episodeofcare-definitions.html#EpisodeOfCare.patient) | Verwijzing naar de Patient. |

## [Condition](https://hl7.org/fhir/R4/condition.html)

| Element | Uitleg |
|---|---|
| [`identifier.value`](https://hl7.org/fhir/R4/condition-definitions.html#Condition.identifier) | Probleemnummer, bijvoorbeeld `PR1`. Eén Condition per DBC. |
| [`code.coding`](https://hl7.org/fhir/R4/condition-definitions.html#Condition.code) | IBD: diagnosecode `922`, `923` (specialisme 0313) of `601`, `602` (specialisme 0318). Overig: diagnoses die niets met IBD te maken hebben, zoals `006` buikklachten en `007` dyspnoe (specialisme 0313), `101` gastroduodenoscopie en `104` colonoscopie (0318), `04` ap/ischemie en `13` follow up na myocardinfarct (0320), `155` refractie-anomalie (0301) en `0201` neoplasma intracerebraal (0330). Het specialisme zit in het systeem: `urn:oid:2.16.840.1.113883.2.4.3.27.15.4.0313`. |
| [`subject`](https://hl7.org/fhir/R4/condition-definitions.html#Condition.subject) | Verwijzing naar de Patient. |

## [Procedure](https://hl7.org/fhir/R4/procedure.html)

| Element | Uitleg |
|---|---|
| [`identifier.value`](https://hl7.org/fhir/R4/procedure-definitions.html#Procedure.identifier) | Verrichtingsnummer, bijvoorbeeld `V1`. |
| [`status`](https://hl7.org/fhir/R4/procedure-definitions.html#Procedure.status) | Altijd `completed`. |
| [`code.coding`](https://hl7.org/fhir/R4/procedure-definitions.html#Procedure.code) | Zorgactiviteit uit het Nictiz-codestelsel [NZa-verrichtingen](https://www.zibs.nl/wiki/Verrichting-v6.0(2024NL)) (systeem `urn:oid:2.16.840.1.113883.2.4.3.27.15.5`): scopie `034620`, `034686`, `034690`, `035582`; soms een andere verrichting (`087090`, `190204`) die buiten het filter valt. |
| [`subject`](https://hl7.org/fhir/R4/procedure-definitions.html#Procedure.subject) | Verwijzing naar de Patient. |
| [`performedPeriod.start`](https://hl7.org/fhir/R4/procedure-definitions.html#Procedure.performed_x_) | Starttijd van de verrichting. |
| [`performedPeriod.end`](https://hl7.org/fhir/R4/procedure-definitions.html#Procedure.performed_x_) | Eindtijd, 20 tot 60 minuten na de start. |

## [Observation](https://hl7.org/fhir/R4/observation.html)

Eén resource voor twee metingen: calprotectine en alcoholgebruik.

| Element | Uitleg |
|---|---|
| [`identifier.value`](https://hl7.org/fhir/R4/observation-definitions.html#Observation.identifier) | Meetnummer, bijvoorbeeld `M1`. |
| [`status`](https://hl7.org/fhir/R4/observation-definitions.html#Observation.status) | Altijd `final`. |
| [`code.coding`](https://hl7.org/fhir/R4/observation-definitions.html#Observation.code) | De meting: [LOINC](https://loinc.org/38445-3) `38445-3` (calprotectine in feces) of [SNOMED CT](https://hl7.org/fhir/R4/snomedct.html) `228273003` (alcoholgebruik). |
| [`subject`](https://hl7.org/fhir/R4/observation-definitions.html#Observation.subject) | Verwijzing naar de Patient. |
| [`effectiveDateTime`](https://hl7.org/fhir/R4/observation-definitions.html#Observation.effective_x_) | Moment van de meting. Bij calprotectine deels binnen 90 dagen vóór een scopie, deels erbuiten. |
| [`valueQuantity.value`](https://hl7.org/fhir/R4/observation-definitions.html#Observation.value_x_) | Alleen calprotectine: uitslag tussen 5 en 1500. |
| [`valueQuantity.comparator`](https://hl7.org/fhir/R4/datatypes-definitions.html#Quantity.comparator) | Alleen calprotectine: `>` als de uitslag de bovengrens (1500) overschrijdt. |
| [`valueQuantity.unit`, `.system`, `.code`](https://hl7.org/fhir/R4/datatypes.html#Quantity) | Alleen calprotectine: eenheid `ug/g`, [UCUM](https://hl7.org/fhir/R4/ucum.html). |
| [`valueCodeableConcept.coding`](https://hl7.org/fhir/R4/observation-definitions.html#Observation.value_x_) | Alleen alcoholgebruik: SNOMED `219006`, `105542008`, `82581004`, `783261004` of [NullFlavor](https://terminology.hl7.org/CodeSystem-v3-NullFlavor.html) `OTH`. |
