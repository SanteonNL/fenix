---
eigenaar: <team of rol, anders "geen">
laatst-gecontroleerd: <DD-MM-JJJJ>
---

<!--
Kopieer dit bestand naar docs/applications/<app>/overview.md.
De mapnaam is de vaste naam van de applicatie: kleine letters en koppeltekens,
zoals indicatoren, dsg of fhir-server.

Doel: wie alleen deze pagina leest, weet precies wat de applicatie hoort te doen,
en waarom. Schrijf in gewone taal.
Een collega die de code niet leest, moet kunnen zeggen: "klopt" of "klopt niet".
Ook een AI-assistent gebruikt deze pagina om te weten wat de code hoort te doen.
Hoe het technisch werkt, staat in de code. Dat schrijven we hier niet op.

Opbouw:
- Een applicatie bestaat uit onderdelen: wat de applicatie kan of doet.
- Elk onderdeel krijgt dezelfde drie vragen.
- Is een onderdeel een keten (een datapijplijn, of de verwerking van een verzoek)?
  Dan krijgt het stappen, elk weer met dezelfde drie vragen.

Alleen regels die iemand moet kunnen toetsen. Geen sectie per functie in de code.

Wordt één onderdeel te groot? Geef het dan een eigen pagina met _template-onderdeel.md.
In dit bestand blijven bij dat onderdeel alleen de eerste twee vragen staan, plus een link.

Haal deze commentaarblokken weg als je klaar bent.
-->

# {Naam van de applicatie}

## Wat doet deze applicatie?

<!-- Eén of twee zinnen: wat doet de applicatie, en wie gebruikt hem? -->

## Waarom bestaat deze applicatie?

<!--
Vanuit welke vraag of welk probleem is hij gebouwd? Wie had die vraag?
Wat zou er gebeuren als hij er niet was?
-->

## Wat valt erbuiten?

<!--
Optioneel. Wat doet deze applicatie bewust níet, terwijl je dat misschien zou
verwachten? Bijvoorbeeld: "slaat zelf geen data op". Weet je niets, haal deze sectie weg.
-->

## Waar draait hij op?

<!--
De resources waar de applicatie op draait, uit leest of naar schrijft.
Link naar hun pagina in docs/resources/, zodat grep op die naam deze pagina vindt.
Bestaat er nog geen pagina? Noem dan alleen de naam.
-->

- **[<Naam>](../../resources/<bestand>.md)**: {waarvoor de applicatie hem gebruikt}.

## Wie of wat hangt van deze applicatie af?

<!--
Alles wat stuk gaat of verkeerd werkt als je de uitvoer of de werking van deze
applicatie verandert: een andere applicatie, een rapport, een team, een afnemer
van een API.

Ook deze lijst werkt maar één kant op: je noemt wie van deze applicatie afhangt,
niet waar deze applicatie zelf van afhangt. Dat staat onder "Waar draait hij op?".

Per regel: wie (met link en "buiten de repo" als dat zo is), wat hij doet met deze
applicatie, en waarop hij leunt, in gewone woorden.

Niemand bekend? Schrijf "Niemand bekend."
-->

- **[<Naam>](<pad>.md)** (<buiten de repo, als dat zo is>): {wat hij doet met deze applicatie}. Leunt op {waarop}.

## Uit welke onderdelen bestaat hij?

<!--
Een onderdeel is iets wat de applicatie doet of kan: een feature, een datapijplijn,
de verwerking van een verzoek, of een regel die overal geldt (zoals de-identificatie).
Eén regel per onderdeel. Elk onderdeel krijgt hieronder een eigen sectie, met
steeds dezelfde drie vragen.
-->

- **{Onderdeel}**: {één zin}

## {Onderdeel}

### Waarom is dit onderdeel nodig?

<!-- Welk probleem lost het op, of welke vraag beantwoordt het? -->

### Wat doet dit onderdeel in grote lijnen?

<!--
Een paar zinnen in gewone taal. Is het een keten? Zet de stappen dan in volgorde:
1. {stap}
2. {stap}
-->

### Wat zijn de sub-onderdelen / stappen / businessregels?

<!--
De afspraken over wat het onderdeel moet doen, zo dat iemand ze kan toetsen.
Eén regel per punt. Schrijf ook op wat er gebeurt als een regel niet past.

Zijn het er veel, of zijn ze precies en belangrijk (zoals bij de-identificatie)?
Geef het onderdeel dan een eigen bestand met _template-onderdeel.md, en zet hier
alleen: "Zie [{onderdeel}.md]({onderdeel}.md)."
-->

- {regel}

## {Groot onderdeel}

<!--
Een onderdeel met een eigen pagina. Maak die pagina met
[_template-onderdeel.md](../_template-onderdeel.md).
Hier blijven alleen de eerste twee vragen staan, plus een link.
-->

### Waarom is dit onderdeel nodig?

{één of twee zinnen}

### Wat doet dit onderdeel in grote lijnen?

{een paar zinnen}

### Wat zijn de sub-onderdelen / stappen / businessregels?

Zie [{onderdeel}.md]({onderdeel}.md).

## Overig

<!--
Optioneel. Wat belangrijk is maar geen onderdeel is, bijvoorbeeld een link naar de
API-specificatie. Weet je niets, haal deze sectie weg.
-->
