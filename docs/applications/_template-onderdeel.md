---
eigenaar: <team of rol, anders "geen">
laatst-gecontroleerd: <DD-MM-JJJJ>
---

<!--
Gebruik dit template voor een onderdeel met veel of belangrijke businessregels.
Kopieer het naar docs/applications/<app>/<onderdeel>.md, zoals dsg/de-identificeren.md.
In overview.md staat bij dit onderdeel dan alleen een link naar deze pagina.

Doel: alle businessregels van dit onderdeel staan hier, nauwkeurig genoeg dat
iemand ze kan toetsen, en dat een AI-assistent er de code mee kan controleren.
Geen techniek: hoe het gebouwd is, staat in de code.

Twijfel je of iets een regel is? Schrijf het op. Een regel te veel is goedkoop,
een vergeten regel niet.

Haal deze commentaarblokken weg als je klaar bent.
-->

# {Naam van de applicatie}: {naam van het onderdeel}

Onderdeel van [{Naam van de applicatie}](overview.md).

## Waarom is dit onderdeel nodig?

<!-- Welk probleem lost het op? Welke wet, afspraak of vraag ligt eronder? -->

## Wat doet dit onderdeel in grote lijnen?

<!-- Een paar zinnen. Is het een keten? Zet de stappen in volgorde. -->

## Sub-onderdelen / stappen / businessregels

<!--
Eén blok per regel. Geef elke regel een nummer, zodat je er in code, tests of
een PR naar kunt verwijzen ("zie R2").

Vragen die je helpen geen regel te vergeten:
- Welke gegevens raakt dit onderdeel? Heeft elk gegeven een regel?
- Wat gebeurt er als een waarde ontbreekt, leeg is of ongeldig is?
- Moet de uitkomst altijd hetzelfde zijn bij dezelfde invoer, ook later?
- Zijn er uitzonderingen: bepaalde ziekenhuizen, datasets of omgevingen?
- Wat mag er nooit gebeuren?

Groepeer de regels als dat helpt, bijvoorbeeld per stap of per gegeven, met een
extra kop (### {Groep}).
-->

### R1 · {Korte naam van de regel}

**Regel:** {wat er moet gebeuren, zo precies dat je het kunt toetsen}

**Waarom:** {de reden}

**Voorbeeld:** `{invoer}` → `{uitkomst}`

**Als het niet past:** {wat er gebeurt bij een ontbrekende, lege of ongeldige waarde}

### R2 · {Korte naam van de regel}

**Regel:**

**Waarom:**

**Voorbeeld:**

**Als het niet past:**

## Overig

<!-- Optioneel. Bijvoorbeeld hoe deze regels getest worden. Weet je niets, haal deze sectie weg. -->
