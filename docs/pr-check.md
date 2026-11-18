# PR-check

Klaar voor een PR? Typ `/pr-check` in Claude Code. Gebruik je een andere AI-assistent, geef hem dan de opdracht hieronder.

---

De PR-check heeft twee delen. Eerst werk je de docs bij, daarna controleer je de hele branch.

**Stap 0: docs bijwerken**

Volg `.agent/documentation/update-docs.md`, ook als de developer `/docs-bijwerken` al gedraaid heeft. Zo zijn de docs bij elke PR bijgewerkt. Docs die eerder in de branch geschreven zijn, kunnen door latere wijzigingen verouderd zijn. Stel de vragen die je niet uit de code kunt halen, en laat de developer de wijzigingen goedkeuren. Is er niets bij te werken, zeg dat dan in één regel en ga door.

**Stap 1: controleren**

Controleer alle wijzigingen op deze branch ten opzichte van `main`, inclusief de docs uit stap 0. Pas in deze stap zelf niets aan.

**Controleer:**

1. **Code:** volgen de wijzigingen `.agent/coding/`?
2. **Docs:** doe de impact-check uit `.agent/documentation/overview.md` en loop de checklist in `docs/README.md` na.
3. **Domeinkennis:** staat er in de wijziging een businessregel, reden of architectuurkeuze die niet in `docs/` staat? Verzin hem niet. Zet hem bij de vragen.
   - Een architectuurkeuze zonder ADR (zie `.agent/documentation/adr.md`)? Zet bij "Te doen": "ADR schrijven met `/adr`".
   - Een ADR in deze PR met status "Voorgesteld"? Zet bij "Te doen": "Is het team akkoord? Zet de ADR dan op Geaccepteerd."
   - Staan er nog regels in het beslissingenlog `.beslissingen/<branch>.md`? Zet ze bij "Vragen": ze zijn na stap 0 nog niet verwerkt.
   - Staat er nog een `{invullen}` op een pagina die deze PR verandert? Zet hem bij "Vragen".
   - **Stille aannames:** gaat code ervan uit dat een lijst, naamconventie of set resources vast is, en maakt deze PR dat onwaar? Bijvoorbeeld code die alleen de bekende brontypes (`local`, `sftp`, `sqldb`, `luscii`) afhandelt, terwijl deze PR een nieuw brontype toevoegt. Of een afnemer die op een tabelnaam in de staging-database leunt, terwijl deze PR de naamgeving van tabellen verandert. Dat is een bevinding, met bewijs zoals hieronder.
4. **Opruimen:** zijn er debug-code, tijdelijke instellingen of TODO's achtergebleven?
5. **Secrets:** staan er wachtwoorden, keys, SAS-tokens, connection strings of persoonlijke config in de wijziging? Altijd **[hoog]**.
6. **Gevoelige data in logs:** kan een logregel of print op welk logniveau dan ook (ook debug) iets tonen van:
   - **patiëntdata:** waarden uit bronbestanden, records of FHIR-resources, ID's, data, of een hele tabel (`log.Printf("%v", record)`, `fmt.Println`, `print(df)`, `head()`);
   - **secrets:** keys, tokens, wachtwoorden, connection strings, of een URL met SAS-token.

   Let ook op `cat`, `message`, `Write-Host`, `echo` en `set -x`. Altijd **[hoog]**.

**Bewijs:**

- Elke bevinding noemt de regel code waar hij op steunt, met `bestand:regel`.
- Een gevolg ("dus gaat X stuk", "dus verdwijnt Y") is een bevinding apart, en heeft eigen bewijs nodig. Lees de code die het gevolg zou veroorzaken, ook als die buiten de diff valt, zoals de condition van een job of de aanroeper van een functie.
- Stel je een oplossing voor, controleer dan eerst of die werkt in de context van de code. Draait de code bijvoorbeeld in een eigen proces, heet iets in de config anders dan in de code, of hangt een waarde af van een andere bron?
- Twijfel je, of steunt een bevinding op een aanname? Dan hoort hij bij "Vragen", niet bij "Te doen".

**Controle door een tweede agent:**

Laat elke bevinding van hoog en middel controleren door een aparte agent (in Claude Code: de Agent-tool), voordat je antwoordt. Geef die agent per bevinding de bewering, het bewijs en de voorgestelde oplossing. Vraag hem om de bevinding te **weerleggen**: klopt de bewering, volgt het gevolg echt uit de code, en werkt de oplossing? De agent past niets aan.

- Weerlegd: haal de bevinding weg.
- Klopt deels, of de ernst is lager: pas de bevinding en de ernst aan.
- Niet zeker: verplaats hem naar "Vragen", met wat er nog uitgezocht moet worden.

Kun je geen aparte agent starten? Doe deze controle dan zelf, als een aparte stap, nadat je alle bevindingen hebt.

**PR-beschrijving:**

Schrijf een beschrijving die de developer in de PR plakt. Een reviewer begint hiermee: hij moet de essentie van de PR zien zonder alle code en docs te lezen.

- **Wat:** groepeer de wijzigingen per onderwerp, met het onderwerp vetgedrukt. Per wijziging één regel. Noem bestanden, functies en parameters bij naam, tussen backticks.
- **Waarom:** per onderwerp het probleem en waarom deze wijziging het oplost. Noem het probleem concreet: de foutmelding, de situatie of het voorbeeld. Haal de reden uit het gesprek met de developer, het beslissingenlog, de commitberichten of `docs/`. Verzin hem niet. Weet je hem niet, zet dan `{invullen: <vraag>}`.
- Kort en in gewone taal, volgens `.agent/documentation/writing-style.md`. Herhaal geen docs: noem de pagina die deze PR verandert.
- Beschrijf wat er na de merge in `main` staat. Tijdelijke code of testcode hoort niet in de beschrijving, maar is een bevinding bij "Te doen".

**Antwoord in dit formaat, en niet meer dan dat:**

````markdown
## Te doen

1. [hoog] <taak in één regel> (<bestand:regel>)
2. [middel] ...
3. [laag] ...

## Vragen

- <wat je niet uit de code kunt halen>

## PR-beschrijving

```markdown
## Wat

**<onderwerp>**
- <wijziging in één regel>

## Waarom

- **<onderwerp>:** <probleem, concreet> <waarom deze wijziging het oplost>

PR-check gedraaid op <korte commit-hash>: <aantal> hoog, <aantal> middel, <aantal> laag open.
```
````

- Sorteer "Te doen" van belangrijk naar minder belangrijk. **Hoog** betekent dat iets stuk gaat, dat er domeinkennis verloren gaat, of dat data in test of productie geraakt kan worden. **Laag** betekent dat het netjes is om te doen.
- Is er niets te doen of te vragen? Schrijf dan "Niets." onder die kop.
- Zet onder de PR-beschrijving één regel: hoeveel bevindingen de tweede agent heeft weggehaald, aangepast of naar "Vragen" verplaatst.
- De laatste regel van de PR-beschrijving (`PR-check gedraaid op ...`) laat de reviewer zien of de PR-check op de laatste versie gedraaid is.
- Geen uitleg en geen complimenten. De gebruiker vraagt zelf door.
