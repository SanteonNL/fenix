---
status: Voorgesteld
datum: <DD-MM-JJJJ>
betrokken: <teams of rollen die de beslissing namen, zoals DA, DE>
vervangt: <ADR-nummer, alleen als deze ADR een eerdere vervangt; anders deze regel weghalen>
---

<!--
Kopieer dit bestand naar docs/adr/<nummer>-<korte-naam>.md, zoals 0001-autotest-containers.md.
Het nummer is het hoogste nummer in deze map plus één. De korte naam gebruikt kleine letters en koppeltekens.

Een ADR (Architecture Decision Record) legt één beslissing vast: waarom we iets zo bouwen, en niet anders.
Schrijf voor een collega die over twee jaar vraagt: "waarom is dit zo?".

Datum: de dag waarop de status voor het laatst veranderde. Zet je de ADR op Geaccepteerd, dan wordt het die dag.

Status:
- Voorgesteld: de ADR is geschreven, maar nog niet afgerond.
- Geaccepteerd: zet je vlak voor de PR, als het team het eens is met de beslissing.
- Vervangen door ADR-<nummer>: een latere beslissing vervangt deze.

Een geaccepteerde ADR pas je niet meer aan, behalve de status. Verandert de beslissing, schrijf dan een nieuwe ADR
met "vervangt: ADR-<oud nummer>", en zet de oude op "Vervangen door ADR-<nieuw nummer>". Verandert maar een deel,
dan vervangt de nieuwe ADR de oude toch helemaal, en herhaalt hij kort wat nog geldt.

Laat een AI-assistent geen context, alternatieven of gevolgen verzinnen. Die komen van mensen. Gebruik /adr in Claude Code:
dan stelt de assistent je gerichte vragen.

Haal deze commentaarblokken weg als je klaar bent.
-->

# ADR-<nummer>: {De beslissing in een paar woorden}

## Context

<!--
Welk probleem moesten we oplossen? Waarom juist toen? Welke eisen of beperkingen speelden mee?

Voorbeeld:
Fouten in de dataverwerking komen in de logs in Log Analytics. Niemand leest die logs dagelijks,
dus een fout blijft liggen tot een gebruiker erover klaagt.
-->

## Beslissing

<!--
Wat hebben we besloten? Eén tot drie zinnen, actief geschreven.

Voorbeeld:
Een Power Automate-flow leest elk uur de foutlogs, en maakt per fout een bug aan op de juiste backlog.
-->

## Overwogen alternatieven

<!--
Elk alternatief dat echt op tafel lag, met de reden waarom we het niet kozen.
Was er geen alternatief? Schrijf dat dan op, met de reden.

Voorbeeld:
- **Een Azure DevOps-pipeline die de logs leest:** de verbinding met de Log Analytics-workspaces
  is lastiger op te zetten dan in Power Automate.
-->

- **{alternatief}:** {waarom niet}

## Gevolgen

<!--
Wat wordt makkelijker, wat wordt moeilijker, en waar moet je voortaan op letten?
Noem ook de nadelen die we bewust accepteren.

Voorbeeld:
- Fouten komen vanzelf als bug bij de juiste persoon terecht.
- De flow staat buiten de repo. Wie de naam van de container instance of de logcodes verandert,
  moet de flow aanpassen.
-->

- {gevolg}

## Raakt

<!--
Links naar de pagina's van de resources en applicaties waar deze beslissing over gaat.
Zet op die pagina's onder "Waarom niet anders?" een link terug naar deze ADR.

Voorbeeld:
- [Power Automate bugflow](../resources/power-automate-bugflow.md)
-->

- [{Naam}](../resources/{bestand}.md)
