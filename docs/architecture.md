# What is FENIX?

**FENIX** stands for **FHIR ENabled Node for Information Exchange**.

It is a FHIR facade: an application deployed inside the hospital that presents itself as a FHIR server to the outside world. Internally, FENIX translates requests into queries against the hospital's source systems (EPD, flat files, APIs), converts the results into FHIR resources, and returns them via standard FHIR operations.

The primary goal of FENIX is to enable secure, controlled data sharing with **HIPS** (Health Information Platform Services), but the same facade can serve any FHIR-compatible consumer — research platforms, quality registries, or other hospital systems.

Key characteristics:

- The hospital retains full data sovereignty — no data is pushed without explicit approval.
- FENIX speaks FHIR outward but connects to non-FHIR source systems inward.
- A dataset export request (YAML + generated FHIR resources) is the unit of governance: it defines what data may be shared, with whom, and how often.



![FENIX hospital overview](images/fenix_hospital_overview.drawio.png)

---

### What does a FHIR request look like?

A FHIR request is a plain HTTP GET to a well-known URL. The server returns JSON. No proprietary protocol, no special client — a browser or `curl` is enough.

**Example: search for female patients born after 1980**

```
GET /fhir/Patient?gender=female&birthdate=gt1980-01-01
Accept: application/fhir+json
```

The server responds with a **Bundle** — a JSON envelope that wraps one or more matching resources:

```json
{
  "resourceType": "Bundle",
  "type": "searchset",
  "total": 2,
  "entry": [
    {
      "resource": {
        "resourceType": "Patient",
        "id": "p-1042",
        "name": [
          {
            "family": "De Vries",
            "given": ["Anna"]
          }
        ],
        "gender": "female",
        "birthDate": "1985-03-22",
        "identifier": [
          {
            "system": "https://ziekenhuis.nl/patientnummer",
            "value": "10042"
          },
          {
            "system": "http://fhir.nl/fhir/NamingSystem/bsn",
            "value": "123456789"
          }
        ]
      }
    },
    {
      "resource": {
        "resourceType": "Patient",
        "id": "p-2187",
        "name": [
          {
            "family": "Janssen",
            "given": ["Sophie"]
          }
        ],
        "gender": "female",
        "birthDate": "1991-07-08",
        "identifier": [
          {
            "system": "https://ziekenhuis.nl/patientnummer",
            "value": "21087"
          },
          {
            "system": "http://fhir.nl/fhir/NamingSystem/bsn",
            "value": "987654321"
          }
        ]
      }
    }
  ]
}
```

Every field has a fixed meaning defined by the FHIR standard — `birthDate`, `gender`, `identifier`, and so on are the same across every FHIR server in the world. That is what makes FHIR useful for data exchange: both sides speak the same language without custom mapping.

FENIX receives this kind of request, translates the filters into queries against the hospital's source systems, and returns the result in exactly this format.

---

### What is a FHIR profile?

The base FHIR standard defines resources like [`Patient`](https://hl7.org/fhir/patient.html) with almost everything optional — it has to, because hospitals worldwide have different requirements. A **profile** is a `StructureDefinition` that constrains a base resource to fit a specific context: it can make fields required, prohibit fields that don't apply, or restrict which values are allowed.

Profiles stack. You start from the international base, derive a national profile, and then derive a hospital-specific profile from that.

```
hl7.org/fhir/Patient          ← international base (everything optional)
        │
        └── nl-core-Patient   ← Dutch national profile (BSN required, Dutch extensions)
                │
                └── ZiekenhuisPatient  ← hospital profile (further restrictions)
```

Each step only describes what *changes* from the parent — this is called the **differential**.

---

#### Example — base Patient StructureDefinition (international)

The base `StructureDefinition` for Patient defines the rules: cardinality (`min`/`max`) and, where applicable, which codes are allowed and how strictly. Below are a few representative fields — the full definition has ~30 elements.

```json
{
  "resourceType": "StructureDefinition",
  "url": "http://hl7.org/fhir/StructureDefinition/Patient",
  "name": "Patient",
  "differential": {
    "element": [
      {
        "id": "Patient.identifier",
        "path": "Patient.identifier",
        "min": 0,
        "max": "*"
      },
      {
        "id": "Patient.gender",
        "path": "Patient.gender",
        "min": 0,
        "max": "1",
        "binding": {
          "strength": "required",
          "valueSet": "http://hl7.org/fhir/ValueSet/administrative-gender"
        }
      },
      {
        "id": "Patient.birthDate",
        "path": "Patient.birthDate",
        "min": 0,
        "max": "1"
      },
      {
        "id": "Patient.maritalStatus",
        "path": "Patient.maritalStatus",
        "min": 0,
        "max": "1",
        "binding": {
          "strength": "extensible",
          "valueSet": "http://hl7.org/fhir/ValueSet/marital-status"
        }
      },
      {
        "id": "Patient.communication.language",
        "path": "Patient.communication.language",
        "min": 1,
        "max": "1",
        "binding": {
          "strength": "preferred",
          "valueSet": "http://hl7.org/fhir/ValueSet/languages"
        }
      }
    ]
  }
}
```

Key observations that explain the profiling choices below:
- `identifier` is `0..*` — the national profile can safely raise it to `1..*`.
- `gender` is already `required`-bound — a profile **cannot** override it with a different ValueSet.
- `maritalStatus` is `extensible` — a profile can tighten or prohibit it.
- `communication.language` is only `preferred` — a profile can tighten it to `required` with a custom ValueSet.

---

#### Deriving a national profile — making a field required

The Dutch **nl-core-Patient** profile extends the base and requires every patient to have a BSN (burgerservicenummer). The `differential` below is the only thing the profile needs to declare — everything else is inherited from the base.

```json
{
  "resourceType": "StructureDefinition",
  "url": "http://nictiz.nl/fhir/StructureDefinition/nl-core-Patient",
  "name": "NlCorePatient",
  "baseDefinition": "http://hl7.org/fhir/StructureDefinition/Patient",
  "differential": {
    "element": [
      {
        "id": "Patient.identifier",
        "path": "Patient.identifier",
        "min": 1
      },
      {
        "id": "Patient.identifier:bsn",
        "path": "Patient.identifier",
        "sliceName": "bsn",
        "min": 1,
        "max": "1",
        "comment": "BSN is mandatory under Dutch law (WGBO).",
        "type": [{ "code": "Identifier" }]
      }
    ]
  }
}
```

`min: 1` means the field is now **required**. A Patient resource that does not include a BSN identifier fails validation against this profile.

---

#### Deriving a hospital profile — prohibiting a field and changing a binding

The hospital extends **nl-core-Patient** further. Two changes:

1. `maritalStatus` is never collected here — **prohibit** it (`max: "0"`) so it cannot accidentally be sent.
2. `communication.language` is `preferred`-bound in the base (any language code is allowed). The hospital tightens this to `required` with a local ValueSet that only contains the languages actually supported in the system.

```json
{
  "resourceType": "StructureDefinition",
  "url": "https://ziekenhuis.nl/fhir/StructureDefinition/ZiekenhuisPatient",
  "name": "ZiekenhuisPatient",
  "baseDefinition": "http://nictiz.nl/fhir/StructureDefinition/nl-core-Patient",
  "differential": {
    "element": [
      {
        "id": "Patient.maritalStatus",
        "path": "Patient.maritalStatus",
        "max": "0",
        "comment": "Not collected; prohibited to prevent accidental disclosure."
      },
      {
        "id": "Patient.communication.language",
        "path": "Patient.communication.language",
        "binding": {
          "strength": "required",
          "valueSet": "https://ziekenhuis.nl/fhir/ValueSet/SupportedLanguages",
          "description": "Only nl-NL, en-GB and de-DE are registered in the EPD."
        }
      }
    ]
  }
}
```

| Change | How | Effect |
|---|---|---|
| Make field required | `min: 1` | Validation fails if the field is absent |
| Prohibit field | `max: "0"` | Validation fails if the field is present |
| Restrict binding | `binding.strength: "required"` + custom `valueSet` | Only codes from that ValueSet are accepted |

Binding strengths go from loose to strict: `example` → `preferred` → `extensible` → `required`. A profile can only tighten a binding, never loosen it.

---

### How does a ConceptMap connect hospital codes to profile codes?

The profile says *which* codes are valid (via the `valueSet` URI in the binding). It does not say how to get there from whatever codes the hospital's source system uses internally. That translation is the job of a **ConceptMap**.

```
EPD source data          ConceptMap               FHIR output
──────────────           ──────────────────────   ──────────────────────────────
"NL"           ───────►  "NL" → "nl-NL"  ───────► "nl-NL"  ✓ valid per profile
"EN"           ───────►  "EN" → "en-GB"  ───────► "en-GB"  ✓ valid per profile
"DU"           ───────►  no mapping       ───────► error: unmapped source code
```

The link between profile and ConceptMap is the **ValueSet URI**:
- The profile binding points to a ValueSet: `https://ziekenhuis.nl/fhir/ValueSet/SupportedLanguages`
- The ConceptMap declares the same URI as its `targetScope`
- FENIX resolves: *"this field requires codes from ValueSet X — find the ConceptMap whose targetScope is X and apply it"*

---

#### Step 1 — the ValueSet (target codes)

The ValueSet lists which codes are valid in the output. This is what the profile binding references.

```json
{
  "resourceType": "ValueSet",
  "url": "https://ziekenhuis.nl/fhir/ValueSet/SupportedLanguages",
  "name": "SupportedLanguages",
  "compose": {
    "include": [
      {
        "system": "urn:ietf:bcp:47",
        "concept": [
          { "code": "nl-NL", "display": "Dutch (Netherlands)" },
          { "code": "en-GB", "display": "English (United Kingdom)" },
          { "code": "de-DE", "display": "German (Germany)" }
        ]
      }
    ]
  }
}
```

---

#### Step 2 — the ConceptMap (the translation rules)

The ConceptMap maps from the EPD's internal codes (source system) to the standard BCP-47 codes that the ValueSet expects (target system). The `targetScope` matches the ValueSet URI in the profile binding — this is what ties them together.

```json
{
  "resourceType": "ConceptMap",
  "url": "https://ziekenhuis.nl/fhir/ConceptMap/EpdTaalNaarBcp47",
  "name": "EpdTaalNaarBcp47",
  "sourceScope": "https://ziekenhuis.nl/fhir/ValueSet/EpdTaalcodes",
  "targetScope": "https://ziekenhuis.nl/fhir/ValueSet/SupportedLanguages",
  "group": [
    {
      "source": "https://ziekenhuis.nl/fhir/CodeSystem/EpdTaalcodes",
      "target": "urn:ietf:bcp:47",
      "element": [
        {
          "code": "NL",
          "display": "Nederlands",
          "target": [
            {
              "code": "nl-NL",
              "display": "Dutch (Netherlands)",
              "relationship": "equivalent"
            }
          ]
        },
        {
          "code": "EN",
          "display": "Engels",
          "target": [
            {
              "code": "en-GB",
              "display": "English (United Kingdom)",
              "relationship": "equivalent"
            }
          ]
        },
        {
          "code": "DE",
          "display": "Duits",
          "target": [
            {
              "code": "de-DE",
              "display": "German (Germany)",
              "relationship": "equivalent"
            }
          ]
        }
      ]
    }
  ]
}
```

The `relationship` field describes how exact the translation is:

| Value | Meaning |
|---|---|
| `equivalent` | The codes mean the same thing |
| `source-is-narrower-than-target` | Source is more specific; target is broader |
| `source-is-broader-than-target` | Source is broader; some detail is lost in translation |
| `not-related-to` | No meaningful relationship — use only to document a deliberate non-mapping |

---

#### Step 3 — how FENIX uses it

When FENIX converts a source row to a FHIR resource:

1. It reads the raw value from the EPD: `"NL"`
2. It checks the profile for `Patient.communication.language` — binding points to `https://ziekenhuis.nl/fhir/ValueSet/SupportedLanguages`
3. It finds the ConceptMap whose `targetScope` matches that URI
4. It looks up `"NL"` in the ConceptMap and gets `"nl-NL"`
5. It writes `"nl-NL"` into the FHIR output — the resource now validates against the profile

If no mapping exists for a source code, FENIX raises a conversion error rather than silently producing an invalid resource.

---

### How does a SearchParameter translate a request filter to a source field?

A FHIR request carries filters as URL parameters:

```
GET /fhir/Patient?language=nl-NL
```

The parameter name `language` is just a string. The server needs to know *which field* on the Patient resource it refers to, and how to evaluate it. That definition lives in a **SearchParameter** resource.

---

#### The SearchParameter resource

A SearchParameter ties a URL parameter name to a FHIRPath expression that points to the field(s) on the resource it searches.

```json
{
  "resourceType": "SearchParameter",
  "url": "https://ziekenhuis.nl/fhir/SearchParameter/Patient-language",
  "code": "language",
  "base": ["Patient"],
  "type": "token",
  "expression": "Patient.communication.language"
}
```

| Field | Role |
|---|---|
| `code` | The URL parameter name (`?language=...`) |
| `base` | Which resource type(s) this applies to |
| `type` | How the value is interpreted: `token` (code), `string`, `date`, `reference`, … |
| `expression` | FHIRPath pointing to the field on the resource |

When the request `?language=nl-NL` arrives, FENIX resolves `language` → SearchParameter → `Patient.communication.language` → token filter on that field.

---

#### How FENIX evaluates a filter — post-conversion on Go structs

FENIX does not push filters down to the source system. Instead it always converts the full source data to FHIR Go structs first, then evaluates the filter directly on those structs using the FHIRPath expression from the SearchParameter. Only matching structs are included in the response Bundle.

```
① Request arrives
   GET /fhir/Patient?language=nl-NL

② SearchParameter resolves the parameter name to a FHIRPath expression
   "language"  →  Patient.communication.language  (type: token)

③ All source rows are loaded and converted to FHIR Patient structs in Go
   ConceptMap (forward): NL → nl-NL,  EN → en-GB,  DE → de-DE

④ The FHIRPath expression is evaluated on each Go struct
   patient.Communication[0].Language.Coding[0].Code == "nl-NL"  →  keep
   patient.Communication[0].Language.Coding[0].Code == "en-GB"  →  drop

⑤ Matching structs are wrapped in a Bundle and returned
```

Because the filter runs after conversion, it operates on FHIR codes already — no reverse ConceptMap lookup is needed. The same struct that was produced by the ConceptMap in step ③ is the one being tested in step ④.

---

#### Filtering by a single code

```
GET /fhir/Patient?language=nl-NL
```

SearchParameter expression `Patient.communication.language` is evaluated on every converted Patient struct. Structs where that field equals the token `nl-NL` are kept; all others are dropped before the Bundle is assembled.

---

#### Filtering by a ValueSet — the `:in` modifier

Instead of a single code you can filter on *all codes in a ValueSet* using the `:in` modifier:

```
GET /fhir/Patient?language:in=https://ziekenhuis.nl/fhir/ValueSet/SupportedLanguages
```

This means: *keep resources whose `communication.language` is any code contained in that ValueSet.*

```
① ValueSet is expanded
   https://.../SupportedLanguages  →  { nl-NL, en-GB, de-DE }

② All source rows are converted to FHIR Patient structs (ConceptMap applied)

③ FHIRPath expression evaluated on each struct
   patient.Communication[x].Language  in  { nl-NL, en-GB, de-DE }  →  keep / drop
```

The ValueSet URI in the request is the same URI that appears as `targetScope` in the ConceptMap — the codes the ConceptMap produces are exactly the codes the ValueSet contains, so the membership check in step ③ always works cleanly.

---

#### Source-level pushdown — not yet implemented

Evaluating filters on Go structs means all source rows are always loaded and converted, even those that will be dropped. For large datasets, pushing the filter down to the source query (SQL `WHERE` clause or API query parameter) before conversion would be more efficient.

This is a planned optimisation. When implemented, it will be configurable per source — sources that support it can use pushdown; others fall back to the post-conversion path described above. Pushdown requires a reverse ConceptMap lookup (FHIR filter code → source code) that is not needed on the current path.

---

# FENIX — annotations explained




![FENIX architecture diagram](images/architecture.drawio.png)

---

## ❶ Dataset export request

The **dataset export request** is the single approved artifact that drives FENIX.
The YAML is the **human-authored source of truth**. From it, FENIX generates the
corresponding FHIR resources automatically — they are never written by hand.

```
oncology-active-2024.yaml          ← human authors this
        │
        └── fenix generate
              ├── Group.json        ← generated: cohort as FHIR Group (Bulk Cohort profile)
              ├── Parameters.json   ← generated: export query as FHIR $export parameters
              └── (stored in Git alongside the YAML, committed in the same PR)
```

---

### The YAML — source of truth

```yaml
# dataset-export-request: oncology-active-2024.yaml

cohort:
  id: oncology-active-2024
  name: Active oncology patients 2024
  filter:
    - resource: Condition
      params: "code=363346000&clinical-status=active"
    - resource: Encounter
      params: "date=ge2023-01-01&class=IMP"

export-query:
  - resource: Patient
    params: ""
  - resource: Observation
    params: "code=363346000&status=final&date=ge2023-01-01"
  - resource: Condition
    params: "code=363346000&clinical-status=active"
  - resource: MedicationStatement
    params: "status=active"

frequency:
  mode: on-demand          # on-demand | scheduled
  cron: ~                  # only set when mode is scheduled, e.g. "0 2 * * *"
  cohort-refresh: dynamic  # dynamic = re-evaluate who is in scope each run
                           # snapshot = patient list frozen at first run
```

---

### Generated — Group.json (cohort as FHIR Group)

The `cohort` block becomes a FHIR **Group** resource using the Bulk Cohort profile.
The `filter` entries map to `member-filter` extensions, one per resource type.
FENIX evaluates these at runtime to resolve the patient list.

```json
{
  "resourceType": "Group",
  "id": "oncology-active-2024",
  "meta": {
    "profile": [
      "http://hl7.org/fhir/uv/bulkdata/StructureDefinition/bulk-cohort"
    ]
  },
  "type": "person",
  "actual": false,
  "name": "Active oncology patients 2024",
  "extension": [
    {
      "url": "http://hl7.org/fhir/uv/bulkdata/StructureDefinition/member-filter",
      "valueString": "Condition?code=363346000&clinical-status=active"
    },
    {
      "url": "http://hl7.org/fhir/uv/bulkdata/StructureDefinition/member-filter",
      "valueString": "Encounter?date=ge2023-01-01&class=IMP"
    },
    {
      "url": "http://hl7.org/fhir/uv/bulkdata/StructureDefinition/members-refreshed",
      "valueDateTime": "2024-01-15T02:00:00Z"
    }
  ]
}
```

> `members-refreshed` is populated by FENIX at runtime, not generated from the YAML.
> It records when the cohort was last evaluated — useful for auditing and debugging.

---

### Generated — Parameters.json (export query as FHIR $export parameters)

The `export-query` block becomes a FHIR **Parameters** resource that maps directly
to the `Group/[id]/$export` operation parameters defined in the Bulk Data Access IG.

```json
{
  "resourceType": "Parameters",
  "id": "oncology-active-2024-export",
  "parameter": [
    {
      "name": "group-id",
      "valueString": "oncology-active-2024"
    },
    {
      "name": "_type",
      "valueString": "Patient,Observation,Condition,MedicationStatement"
    },
    {
      "name": "_typeFilter",
      "valueString": "Observation?code=363346000&status=final&date=ge2023-01-01"
    },
    {
      "name": "_typeFilter",
      "valueString": "Condition?code=363346000&clinical-status=active"
    },
    {
      "name": "_typeFilter",
      "valueString": "MedicationStatement?status=active"
    },
    {
      "name": "_outputFormat",
      "valueString": "application/fhir+ndjson"
    }
  ]
}
```

> `_typeFilter` is the standard Bulk Data IG parameter that scopes which resources
> within a type are included — it is the FHIR representation of the `export-query` entries.
> `Patient` has no filter so it appears only in `_type`, not in `_typeFilter`.

---

### All three files committed together

```
requests/
└── oncology-active-2024/
    ├── oncology-active-2024.yaml       ← human authors this
    ├── Group.json                       ← generated by fenix generate
    └── Parameters.json                  ← generated by fenix generate
```

The generated files are committed into Git in the same PR as the YAML.
This means reviewers can read either the YAML (human-friendly) or the FHIR JSON
(machine-exact), and the CI pipeline can validate both.
The FHIR JSON is what FENIX actually loads at runtime.

---

## ❷ Approval

Approval happens at two levels: **central** (GitHub, once per version) and
**local** (Hospital Approval Service, before every run).

### Central approval — GitHub PR

Governs the *definition* of the request. Required whenever the YAML is
created or changed.

```
fenix generate                         generates Group.json + Parameters.json from YAML
        │
        ▼
PR opened (YAML + Group.json + Parameters.json)
        │
        ▼
CODEOWNERS review
  data steward + privacy officer approve
  checks: cohort scope, exported fields, frequency justification
        │
        ▼
CI checks (automated)
  YAML schema valid · FHIR params known to FENIX
  column allowlist · no free-text · no direct identifiers
        │
        ▼
merge → available in FENIX runtime · audit trail locked
```

### Local approval — Hospital Approval Service

Governs *execution*. Required before every single run, regardless of frequency mode.

| Mode | How local approval works |
|---|---|
| `on-demand` | Staff member initiates the run in the local UI — this act is the approval. |
| `scheduled` | Cron proposes a run. Staff member (or configured auto-approve rule) confirms before FENIX executes. |

> **Central approval** defines what is *allowed*.
> **Local approval** decides what *actually runs*.
> The hospital retains full control over when data leaves the EPD.

---

## ❸ Loading

Loading is the step that pulls raw data from source systems into the
**staging database**, where it becomes queryable SQL. Only after loading
does the converter run its SQL-to-FHIR queries.

![Loading layer diagram](images/architecture_loading.drawio)

---

### Layer 1 — Sources

The origin systems: **Luscii**, **SIM**, **HIX**. These are external to FENIX
and are never written to — they are read-only data suppliers.

---

### Layer 2 — Connectors

How data leaves a source system:

| Connector | Description |
|---|---|
| `API` | REST call to an external service (e.g. Luscii Vitals API) |
| `Flat file` | CSV files on disk (e.g. SIM export) |
| `Database` | Direct SQL connection to a source database |

---

### Layer 3 — Source (interface)

Each connector maps to a `Source` implementation (`internal/source`).
The `type:` field in config selects which implementation to use — this is a
per-source choice, independent of environment.

| Implementation | Config `type` | What it does |
|---|---|---|
| `LusciiSource` | `api` | Calls the live REST API; typed deserialisation per source |
| `LocalSource` | `local` | Reads all files from `dir`; `.json` and `.csv` auto-detected by extension |

**`LocalSource` is generic.** It handles both JSON (API-shaped) and CSV files from
the same directory. Switching from live to local — or adding a new source like SIM —
requires no new code, only a `dir` entry in config.

```yaml
sources:
  luscii:
    type: local                   # api | local — per source, not per environment
    dir: "test/data/luscii"       # .json files → JSON parser; .csv files → CSV parser

  hix_patients:                   # flat file source — same type, different format
    type: local
    dir: "data/hix"
    delimiter: ";"

  luscii_live:                    # switch one source to live API without touching others
    type: api
    base_url: "https://vitalsapi.luscii.com"
    api_key: ""
```

Adding a new API source requires one Go file implementing the `Source` interface
and one `case` in `buildSource()`.

---

### Layer 4 — Staging database

All loaders write into a shared **staging database** (SQLite by default).
The staging database is a transient, per-run store — it exists only to make
raw source data queryable by the converter's SQL files.

| Config | Behaviour |
|---|---|
| `path: ""` *(default)* | In-memory SQLite — fast, no file written, lost after the run |
| `path: output/staging.db` | Persisted SQLite — survives the run, useful for debugging SQL queries |
| `type: postgres` | PostgreSQL — for shared or production staging environments |

```yaml
database:
  type: sqlite
  # path: ""                    # default: in-memory
  # path: output/staging.db     # persist for debugging
```

---

### Layer 5 — Converter

After loading, the converter reads `.sql` files from `queries/<sourceName>/`
and executes each against the staging database. Each SELECT produces rows
that are assembled into FHIR resources (see the SQL format in the help output).

The **no-transformation path** (FHIR JSON connector) bypasses the staging
database entirely — data that is already valid FHIR JSON is passed through
directly.