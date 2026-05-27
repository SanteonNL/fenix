**Werkpakket canvas digitaliseringsstrategie Santeon**
*EHDS | IBD use case | HEALTH-RI*

---

**1. Waarom bestaat dit werkpakket?**

De European Health Data Space (EHDS) verplicht ziekenhuizen vanaf 2027 secundair gebruik van gezondheidsdata te faciliteren. Dit wordt in Nederland geconcretiseerd via een samenwerking tussen HEALTH-RI (secundair gebruik, onderzoeksinfrastructuur) en Cumuluz (brede data-beschikbaarheid conform open standaarden).

IBD is door HEALTH-RI en Cumuluz geselecteerd als prioritaire use case. Vanuit uitkomstgerichte zorg zijn al eerder stappen gezet om landelijke KPI's rondom IBD eenduidig te definiëren — een traject waarbij Santeon al actief betrokken was. Dit biedt een solide basis: definities zijn grotendeels uitgekristalliseerd, wat de technische ontsluiting versnelt. Deelname positioneert Santeon als koploper in gestandaardiseerde data-uitwisseling voor wetenschappelijk onderzoek en kwaliteitsverbetering.

---

**2. Wat is de doelstelling?**

Santeon-ziekenhuizen leveren gestandaardiseerde, herbruikbare IBD-data aan de HEALTH-RI onderzoeksinfrastructuur, conform EHDS-vereisten en FHIR R4-standaarden. De use case dient als blauwdruk voor toekomstige zorgpad-ontsluiting binnen de digitaliseringsstrategie.

*Aanverwante initiatieven (niet-prioritair):* Johnson & Johnson en IKNL bewegen in dezelfde richting — maar rondom andere use cases, zoals longkanker. De koppeling met IKNL is extra relevant omdat Santeon NKR-data (Nationale Kanker Registratie) al verwerkt binnen de reguliere HIPS-datastromen, en IKNL zelf de NKR FHIR-compliant wil maken. Afstemming met deze partijen is waardevol maar volgt na de primaire HEALTH-RI-ontsluiting.

*Vraag: Hoe past dit werkpakket binnen welk onderdeel van de digitaliseringsstrategie?*

---

**3. Wie zijn betrokken?**

- Trekker: Data Engineering team Santeon (HIPS/FENIX)
- Ziekenhuizen: MZH, MST?, ? 
- Externe partners: HEALTH-RI, Cumuluz, Nictiz, IKNL
- Stakeholders: VBHC teams, bestuur

---

**4. Hoe pakken we dit aan?**

Per Santeon-ziekenhuis wordt in kaart gebracht hoe FHIR-ontsluiting van IBD-data gerealiseerd wordt. FENIX — de vendor-neutral FHIR-facade van Santeon — is hierbij één van de mogelijke oplossingen, naast ziekenhuiseigen FHIR-servers of andere infrastructuur.

De modellering van data geschiedt in samenwerking met Nictiz, waarbij het huidige Santeon informatiemodel (SIM) FHIR-ready wordt gemaakt via het SIM-on-FHIR traject. Zowel FENIX als SIM-on-FHIR worden open source ingericht, zodat opschaling buiten de Santeon-ziekenhuizen — in tegenstelling tot de huidige situatie — ook mogelijk is.

Voor validatie, data-governance en kwaliteitsbewaking wordt aangesloten op de bestaande systematiek zoals die binnen HIPS is ingericht. Dit voorkomt dubbel werk en borgt consistentie met lopende processen.

---

**4b. Hoe organiseren we dit?**

*Samenwerkingsvorm — pilotaanpak met twee sporen:*

- **Spoor 1 — FENIX:** minimaal één van MZH of MST neemt deel als koplopers; zij zijn al aangesloten op de DatasetGenerator (de voorloper van FENIX) en kunnen snel starten.
- **Spoor 2 — alternatieve ontsluiting:** één huis dat geen gebruik wil maken van FENIX doet mee met een eigen of alternatieve FHIR-oplossing (bijv. [naam oplossing]). Dit toetst de blauwdruk los van FENIX en maakt het model robuuster.

*Werkmethodiek:* iteratief en pilotgericht — eerst werkende ontsluiting bij twee huizen, dan opschaling naar overige Santeon-ziekenhuizen.

*Governance:* inrichting langs de bestaande HIPS-governance; geen parallelle structuur, maar aansluiting op bestaande beheercommissie en besluitvormingslijnen. Uitzondering: extra betrokkenheid van CIO's en BI-verantwoordelijken per ziekenhuis — bij voorkeur geborgd via een stuurgroep of soortgelijk overleg.

*Relatie externe initiatieven:* afstemming met HEALTH-RI over aansluitvereisten; contact met IKNL over NKR-FHIR traject als aanverwant initiatief.

---

**5. Wanneer?**

- Q1 2026: inventarisatie databeschikbaarheid en technische aanpak per ziekenhuis
- Q2 2026: technische inrichting FHIR-ontsluiting IBD en SIM-on-FHIR
- Q3 2026: eerste testleveringen HEALTH-RI
- Q4 2026: structurele aanlevering alle Santeon-ziekenhuizen
- 2027: EHDS-compliance gereed

---

**6. Randvoorwaarden**

- Beschikbaarheid en kwaliteit van IBD-data in bronsystemen per ziekenhuis
- Inzicht in de technische ontsluiting die elk ziekenhuis voor ogen heeft (FENIX, eigen FHIR-server of anders)
- Bestuurlijk commitment Santeon-ziekenhuizen voor deelname
- Aansluiting op HEALTH-RI infrastructuur en dataprotocollen
- Capaciteit data engineering team voor parallelle uitvoering naast bestaande werkzaamheden

---

**7. Risico's**

- Datakwaliteit verschilt sterk per ziekenhuis —  harmonisatiewerk nodig
- EHDS-regelgeving én de uitwerking ervan voor Nederland nog in ontwikkeling — vereisten kunnen wijzigen en nationale implementatiekeuzes zijn nog niet vastgesteld
- Juridische en privacyvraagstukken rondom secundair gebruik: anders dan aanlevering aan HIPS gaat data hier naar een externe partij (HEALTH-RI), wat aanvullende governance en toestemmingsvraagstukken met zich meebrengt

---

**8. Resultaat / deliverables**

- Herbruikbare FHIR-gebaseerde IBD-dataset conform EHDS en Nictiz-profielen, gerealiseerd in samenhang met het SIM-on-FHIR traject
- Werkende FHIR-ontsluiting per Santeon-ziekenhuis (via FENIX of alternatief)
- Open source beschikbaar gestelde FENIX en SIM-on-FHIR
- Blauwdruk voor ontsluiting van volgende zorgpaden
- Santeon als aantoonbaar EHDS-ready ziekenhuisgroep

---

**Koppeling naar vervolgwerkpakket**

Gelijktijdig met dit werkpakket dient te worden gestart met de verkenning van het FHIR-analytisch landschap. Wanneer data eenmaal conform FHIR beschikbaar is, ontstaat de vraag hoe deze analytisch ontsloten en bevraagd kan worden — bijvoorbeeld voor kwaliteitsregistraties, dashboards en onderzoek. Dit vraagt om een apart maar parallel werkpakket dat de FHIR-analytische infrastructuur uitwerkt.

