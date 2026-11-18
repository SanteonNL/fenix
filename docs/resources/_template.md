---
eigenaar: <team of rol, anders "geen">
laatst-gecontroleerd: <DD-MM-JJJJ>
---

<!--
Kopieer dit bestand naar docs/resources/<naam>.md.
De bestandsnaam is de vaste naam van de resource: kleine letters, koppeltekens en de
naam waaronder collega's hem kennen, zoals indicatoren-container-instances.md.
Resources die bij elkaar horen delen één pagina.

Kop:
- eigenaar: wie je belt als je twijfelt. Liefst een team of rol, geen persoon.
- laatst-gecontroleerd: de datum waarop iemand deze pagina naast de werkelijkheid legde.

Schrijf kort. Alleen "Wie of wat hangt van deze resource af?" moet compleet zijn.
Haal deze commentaarblokken weg als je klaar bent.
-->

# {Naam van de resource}

## Gedefinieerd in

<!-- Waar de resource in de repo wordt aangemaakt of uitgerold, of "Buiten de repo: <waar>". -->

## Wat is het en waar is het afhankelijk van?

<!--
Eén of twee zinnen: wat is het en wat doet het?
Noem de echte naam in Azure (of elders), zodat grep op die naam deze pagina vindt.
Geen instellingen of parameters: die staan in de code.
Staat de resource buiten de repo? Noem dan ook waar hij op leunt, want geen
AI-assistent kan dat zien. Voor resources in de repo staat dat in de code.
-->

## Waarom bestaat deze resource?

<!-- Waarom bestaat deze resource, en waarom ziet hij er zo uit? -->

## Waarom niet anders?

<!--
Optioneel. Alleen een link naar de ADR (architectuurbeslissing) die uitlegt waarom deze resource zo is
gebouwd, zoals: Zie [ADR-0001](../adr/0001-autotest-containers.md).
Is er een bewuste keuze gemaakt, maar nog geen ADR? Maak er dan een met /adr in Claude Code.
Geen bewuste keuze? Haal deze sectie weg.
-->

Zie [ADR-<nummer>](../adr/<nummer>-<naam>.md).

## Wie of wat hangt van deze resource af?

<!--
Alles wat stuk gaat of verkeerd werkt als je deze resource verandert: een applicatie,
een andere resource, iets buiten de repo (een flow, een rapport) of een team.

Deze lijst werkt maar één kant op. Je noemt wie van deze resource afhangt, niet
waar deze resource zelf van afhangt.

Voorbeeld: de Power Automate bugflow leest de logs van de indicatoren-instance.
- Op indicatoren-container-instances.md staat de bugflow in deze lijst: verander je
  de naam van de instance, dan vindt de flow geen logs meer.
- Op power-automate-bugflow.md staat de instance níet in deze lijst: de instance
  draait gewoon door als de flow stuk is.

Per regel:
- wie: link naar zijn pagina als die bestaat, en zet erbij of hij buiten de repo staat;
- wat hij doet met deze resource;
- waarop hij leunt, in gewone woorden: de naam, de logcodes, de map 20_verified.

Noem applicaties en resources, geen functies of scripts. Welke code op deze resource
leunt, vraag je per PR aan een AI-assistent.

Niemand bekend? Schrijf "Niemand bekend."
-->

- **[<Naam>](<bestand>.md)** (<buiten de repo, als dat zo is>): {wat hij doet met deze resource}. Leunt op {waarop}.

## Regels

<!--
Optioneel. Afspraken over de resource zelf die iemand moet kunnen toetsen, zoals:
in welke omgevingen hij bestaat, een naamconventie, een bewaartermijn, of wanneer
zijn inhoud ververst of verwijderd wordt. Eén regel per punt, met de reden als je die weet.
Regels over wat een applicatie doet, horen in de overview van die applicatie.
Geen regels? Haal deze sectie weg.
-->

- {regel}

## Eventueel extra informatie, bijvoorbeeld over testen van de resource
