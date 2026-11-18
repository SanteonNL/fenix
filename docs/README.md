# Documentatie

Deze map legt vast wat je niet uit de code kunt halen:

- **wie of wat op een resource leunt**, en waarom die resource bestaat;
- **wat een applicatie hoort te doen**, en waarom: de businessregels;
- **waarom we iets zo hebben gebouwd**, en niet anders: de architectuurbeslissingen.

Hoe iets technisch werkt, staat in de code. Dat herhalen we hier niet.

De andere pagina's in `docs/`, zoals `fenix_architecture.md`, zijn van vóór deze indeling. Raak je er een aan, zet hem dan om naar de indeling hieronder.

## Waar staat wat

| Vraag                                                     | Plek                                 |
| --------------------------------------------------------- | ------------------------------------ |
| Wie of wat leunt op deze resource, en waarom bestaat hij? | `docs/resources/`                    |
| Wat moet deze applicatie doen, en waarom?                 | `docs/applications/<app>/`           |
| Waarom hebben we het zo gebouwd, en niet anders?          | `docs/adr/`                          |
| Hoe werkt het, en welke code leunt erop?                  | De code. Vraag het een AI-assistent. |

```text
docs/
  README.md                    afspraken en zoeken (dit bestand)
  pr-check.md                  de opdracht achter /pr-check
  resources/
    _template.md               begin hier voor een nieuwe resource
    <resource>.md              één pagina per resource
  applications/
    _template.md               begin hier voor een nieuwe applicatie
    <app>/overview.md          de onderdelen, met per onderdeel de businessregels
    _template-onderdeel.md          begin hier als een onderdeel een eigen pagina krijgt
    <app>/<onderdeel>.md            alleen als een onderdeel te groot wordt voor overview.md
  adr/
    _template.md               begin hier voor een nieuwe architectuurbeslissing
    <nummer>-<naam>.md         één beslissing per bestand, zoals 0001-config-schema.md
```

## Wat raakt mijn feature?

1. **Zoek op de naam van wat je verandert**, vanuit de root van de repo:

   ```bash
   grep -ril "luscii" docs/
   grep -ril "staging" docs/
   ```

   Je vindt de pagina zelf en elke pagina die ernaar verwijst.

2. **Lees op elke treffer "Wie of wat hangt van deze resource af?".** Daar staat wie je verandering kan breken, en waarop hij leunt.

3. **Vraag een AI-assistent welke code leunt op wat je verandert.** Bijvoorbeeld: "Welke code in deze repo leunt op de namen van de tabellen in de staging-database?" Controleer het antwoord.

**Geen treffers betekent niet dat er niets geraakt wordt.** Het betekent alleen dat de docs het niet weten. Vind je toch een afhankelijkheid, schrijf hem dan op.

## Afspraken

1. **De code zegt wat een resource nodig heeft. De pagina zegt wie hem nodig heeft en waarom hij bestaat.**
2. **Eén resource, één vaste naam: de bestandsnaam van zijn pagina.** Gebruik kleine letters, koppeltekens en de naam waaronder collega's de resource kennen, zoals `staging-database.md`. Zet de echte naam, zoals die in de config of op de server staat, op de pagina zelf.
3. **Noem je een resource of applicatie, link dan naar zijn pagina.** Zo vindt grep op de naam ook elke pagina die ernaar verwijst.
4. **Onder "Wie of wat hangt van deze resource af?" staan applicaties en resources, geen functies of scripts.** Zet bij elke regel waar hij op leunt, in gewone woorden: de naam, een tabelnaam zoals `pja_journeys`, een configveld. Welke code ergens op leunt, vraag je per PR aan een AI-assistent.
5. **Wat buiten de repo staat, moet in de docs staan.** Een AI-assistent kan een Power Automate-flow of een rapport niet zien. Maak of wijzig je zoiets, doe daarna een kleine docs-PR. Neem die stap op in het ticket.
6. **Docs in dezelfde PR als de code.** Verandert er iets aan wie op een resource leunt, of aan een businessregel? Pas de pagina aan in dezelfde PR. Loop daarvoor de [checklist](#checklist-voor-je-pr) hieronder na. Een AI-assistent kan de wijziging voorbereiden; jij controleert hem.
7. **Schrijf als je het aanraakt.** Geen inhaalslag. Een korte pagina is beter dan geen pagina.
8. **Houd het overzichtelijk.** Geen pagina per functie en geen techniek.
9. **Een architectuurbeslissing krijgt een ADR.** Is een keuze moeilijk terug te draaien, raakt hij meerdere applicaties of resources, of waren er echte alternatieven? Leg hem dan vast in `docs/adr/`. Typ in Claude Code `/adr <onderwerp>`, bijvoorbeeld `/adr config-schema`. Weet je het onderwerp niet precies, typ dan alleen `/adr`: de AI-assistent stelt dan op basis van je branch onderwerpen voor, en jij kiest er een of noemt zelf een ander. Daarna stelt hij je vragen. De antwoorden komen van jou. Op de pagina's staat onder "Waarom niet anders?" alleen een link naar de ADR. Vlak voor de PR zet je de status op "Geaccepteerd", als het team het eens is.

## Checklist voor je PR

Loop deze lijst na voordat je een PR aanmaakt. Je mag hem in de beschrijving van je PR plakken en afvinken. Laat daarnaast een AI-assistent de [PR-check](pr-check.md) doen: typ `/pr-check` in Claude Code. Die werkt eerst de docs bij met de domeinkennis uit je branch, en controleert daarna alles. Plak de laatste regel van de PR-check in de beschrijving van je PR.

Tussendoor de docs bijwerken kan met `/docs-bijwerken`.

```markdown
- [ ] Gezocht op de namen van wat ik verander (`grep -ril "<naam>" docs/`) en "Wie of wat hangt van deze resource af?" gelezen.
- [ ] Afhankelijkheden afgegaan en gecontroleerd op impact van mijn verandering.
- [ ] Resource-pagina bijgewerkt of gemaakt, als er iets veranderd is aan wie op een resource leunt of waarom hij bestaat.
- [ ] Overview van de applicatie bijgewerkt, als er een businessregel veranderd is.
- [ ] ADR geschreven voor elke architectuurbeslissing in deze PR, en op "Geaccepteerd" gezet.
- [ ] Buiten de repo iets gemaakt of gewijzigd dat op een resource leunt? Maak een docs-PR en zet het PR-nummer in het ticket.
```

## Een nieuwe pagina

- **Resource:** kopieer `resources/_template.md` naar `resources/<naam>.md`. Resources die bij elkaar horen delen één pagina.
- **Applicatie:** kopieer `applications/_template.md` naar `applications/<app>/overview.md`. Wordt één onderdeel te groot voor `overview.md`, kopieer dan `applications/_template-onderdeel.md` naar `applications/<app>/<onderdeel>.md`. In `overview.md` blijven bij dat onderdeel alleen de eerste twee vragen staan, plus een link.

- **Architectuurbeslissing:** typ `/adr <onderwerp>` in Claude Code, bijvoorbeeld `/adr config-schema`, of alleen `/adr` voor voorstellen. Zonder Claude Code kopieer je `adr/_template.md` naar `adr/<nummer>-<naam>.md`.

De templates leggen uit wat in elke sectie hoort.
