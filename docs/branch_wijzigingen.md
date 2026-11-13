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

## Open punten en bevindingen

- **Luscii `json_options`:** de sleutel is daar de `table` van het endpoint, bij `local`/`sftp` de bestandsnaam. Advies: zet `primary_key_field` en `children` in het endpoint zelf, zodat er geen sleutel meer gekoppeld hoeft te worden. Hoort bij punt 9.
- **Bug luscii:** sleutels in `json_options` worden naar kleine letters omgezet, `endpoint.table` niet. Bij een tabelnaam met hoofdletters worden de instellingen stil genegeerd.
- **Velden voor later:** `fhir.profile`, `output.type: datalake` en `output.datalake` doen nog niets; bewust behouden, in de beschrijving gemarkeerd als "nog niet in gebruik".
- **Opruimen (andere PR):** `fenix/.claude/settings.json` bevat persoonlijke paden van een collega.
