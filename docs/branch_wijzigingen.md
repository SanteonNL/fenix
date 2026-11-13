# Wijzigingen op `feature/pc_staging_layer_feedback`

Werklogboek van wat er op deze branch verandert en waarom, als basis voor de PR-samenvatting.
De feedbackpunten verwijzen naar [fenix_feedback.md](fenix_feedback.md).

## Punt 6: configuratie begrijpelijk maken met een JSON-schema

**Probleem:** uit het configbestand is niet af te lezen welke velden bij welk `type` horen, welke waarden zijn toegestaan of wat `{}` betekent. Verkeerde of verkeerd geplaatste velden werden stil genegeerd.

**Wijzigingen:**

| Wat | Waar | Waarom |
|---|---|---|
| Schemagenerator in een aparte Go-module | `tools/go.mod`, `tools/genschema/main.go` | De schema-library (invopop/jsonschema) is alleen een ontwikkeltool en mag geen afhankelijkheid van het Fenix-programma worden (minder libraries = minder beveiligingsrisico). Gecontroleerd: `go list -deps ./cmd/fenix` bevat geen invopop. |
| Gegenereerd schema | `fenix-config.schema.json` | Eén bron van waarheid: het schema komt uit de config-structs, dus het kan niet uit de pas lopen met de code (zolang het opnieuw gegenereerd wordt). |
| `make schema` en `//go:generate` | `Makefile`, `config/config.go` | Eén vast commando om het schema te vernieuwen, vindbaar naast de structs. |
| Nederlandse veldbeschrijvingen met voorbeeld, beginnend met "Verplicht"/"Optioneel" | `config/config.go` | Worden de hovertekst in VS Code (Red Hat YAML-extensie). Nederlands omdat de gebruikers Nederlands zijn. |
| Toegestane waarden (`enum`) en verplichte velden (`required`) als struct-tags | `config/config.go` | Editor toont keuzelijst en markeert ontbrekende of foute waarden. Standaard is een veld optioneel; alleen `jsonschema:"required"` maakt het verplicht. |
| Regels per brontype (verplichte en verboden velden) | `tools/genschema` (`fieldsBySourceType`) | `SourceConfig` is één platte struct voor alle typen; het schema markeert nu bijvoorbeeld `dir` bij een `sftp`-bron. Verhuist naar de structs zodra elk type een eigen struct krijgt (punt 9). |
| Handmatige schemabeschrijving van `json_options` | `tools/genschema` | Het veld is in Go een ongetypeerde map; de generator kan de structuur niet zien. Wordt overbodig met punt 9. |
| Schema gekoppeld via eerste regel van het voorbeeldbestand | `config/config.example.yaml` | Geen wijziging in `.vscode/settings.json` nodig. |
| `type: sqlserver` → `sqldb` | `config/config.example.yaml` | Bug op `main`: het type `sqlserver` bestaat niet, de bron werd overgeslagen. Het schema vond deze fout direct. |
| Commentaarblok van 9 regels over `json_options` verwijderd | `config/config.example.yaml` | Uitleg staat nu in de hovertekst; het blok was ook deels onjuist ("generated FK column"). |
| Strikt inlezen van de config (`KnownFields(true)`) | `config/config.go` (`decodeStrict`), `config/config_test.go` | Een onbekende of verkeerd gespelde veldnaam stopt Fenix bij het opstarten met een duidelijke fout, in plaats van stil genegeerd te worden. Controleert niet de inhoud van `json_options`, type-specifieke velden of verplichte velden; dat volgt met punt 9. |
| Regel "schema opnieuw genereren, ook bij alleen een beschrijving" | `CLAUDE.md`, `config/config.go`, `tools/genschema` | De commentaren bij de structs zijn de hovertekst; een vergeten `make schema` geeft verouderde uitleg. CI-controle volgt in punt 11. |

**Getest:** `go vet`, `go test ./config/`; het voorbeeldbestand en de DSG-template uit HipsETL zijn geldig volgens het schema; een bestand met een typfout, een verkeerd geplaatst veld en een ontbrekend verplicht veld wordt afgekeurd.

**Niet getest:** de hovertekst in VS Code zelf.

## Punt 7: `id_field` wordt `primary_key_field`, en sleutelfouten laten de tabel mislukken

**Probleem:** `id_field` is in werkelijkheid de primaire sleutel (PRIMARY KEY, upsert-sleutel én de waarde waarmee child-rijen naar hun parent verwijzen), maar de naam laat dat niet zien. Daarnaast faalden sleutelfouten stil: zonder sleutel kregen alle child-rijen `"<nil>"` als verwijzing, en dubbele sleutels lieten rijen wegvallen met alleen een logregel.

**Wijzigingen:**

| Wat | Waar | Waarom |
|---|---|---|
| `id_field` hard hernoemd naar `primary_key_field` (config, Go-veld `PrimaryKeyField`, logs) | `source/local`, `source/luscii`, `config/config.example.yaml`, `tools/genschema` | De naam zegt nu wat het veld doet. Bewust geen overgangsperiode: de oude naam geeft een fout met de hint "renamed to primary_key_field". |
| `json_options` wordt streng geparsed | `source/local/source.go` (`ParseJSONOptions`) | Onbekende opties, de oude naam en `children` zonder `primary_key_field` stoppen de bron bij het opstarten met een duidelijke fout (voorheen stil genegeerd of `"<nil>"`-koppelingen). |
| Sleutelcontrole vóór het schrijven | `source/local/loader.go` (`checkPrimaryKeys`) | Een record zonder sleutelwaarde (ontbrekend, null of leeg) of een dubbele sleutel laat de hele tabel (en zijn children) mislukken, met aantallen en voorbeeldwaarden in de fout. De bestaande tabel blijft dan ongewijzigd. Andere tabellen en bronnen laden door. |
| `id_field` per Luscii-endpoint verwijderd | `config/config.go`, `config/sources.go`, `source/luscii/source.go` | Was al verouderd; de sleutel staat in `json_options.<table>.primary_key_field`. `luscii.New()` krijgt `jsonOptions` als parameter in plaats van `IDField` per endpoint. Het commentaar dat HipsETL `New()` aanroept was onjuist: HipsETL gebruikt alleen `source/sftp`. |
| Schema: `primary_key_field` verplicht zodra `children` gevuld is | `tools/genschema`, `fenix-config.schema.json` | De editor markeert de fout al tijdens het typen. |
| Tests voor parsing en loader | `source/local/loader_test.go` | Eerste tests voor de loader: koppeling child → parent, weigeren van ontbrekende/dubbele sleutels zonder de tabel aan te tasten, en de configfouten. |
| `any` in plaats van `interface{}` in nieuwe en herschreven code | `source/local` | Leesbaarder, zelfde type. Bestaande code niet aangepast (richtlijn: geen refactor alleen voor stijl). |

**Gevolg voor HipsETL:** `config/template_fenix.config.yaml` (branch `feature/th_test_fenix_import`) en `docs/datasource_queries/PJA/README.md` gebruiken `id_field`. Bij het bijwerken van de Fenix-versie in HipsETL moeten die naar `primary_key_field`, anders stopt de PJA-bron met een fout.

**Getest:** `go vet`, `go test ./config/ ./source/local/`; schema keurt het voorbeeldbestand goed en een bestand met `id_field` en met `children` zonder sleutel af.

**Bewust niet in punt 7:** het opslagformaat van sleutels (`"111.0"`, `"1.234567e+06"`, afronding boven 16 cijfers). Dezelfde oorzaak als punt 8 en daar in één keer op te lossen. Tot die tijd gebruiken sleutelcontrole en koppeling hetzelfde formaat (`primaryKeyText`), zodat ze wel consistent zijn met elkaar.

## Open punten en bevindingen

- **Luscii `json_options`:** de sleutel is daar de `table` van het endpoint, bij `local`/`sftp` de bestandsnaam. Advies (akkoord van Pepijn): zet `primary_key_field` en `children` in het endpoint zelf, zodat er geen sleutel meer gekoppeld hoeft te worden. Hoort bij punt 9.
- **Optioneel uit punt 7, niet gedaan:** kleinkind-rijen ook de sleutel van het hoogste record geven, zodat je niet via elke tussenlaag hoeft te joinen.
- **Opmaak:** `source/local/source.go` was op `main` al niet `gofmt`-schoon (dubbele lege regel); niet aangepast.
- **Bug luscii:** sleutels in `json_options` worden naar kleine letters omgezet, `endpoint.table` niet. Bij een tabelnaam met hoofdletters worden de instellingen stil genegeerd.
- **Velden voor later:** `fhir.profile`, `output.type: datalake` en `output.datalake` doen nog niets; bewust behouden, in de beschrijving gemarkeerd als "nog niet in gebruik".
- **Opruimen (andere PR):** `fenix/.claude/settings.json` bevat persoonlijke paden van een collega.
