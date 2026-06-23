# Release Process — FENIX

FENIX gebruikt [GoReleaser](https://goreleaser.com/) voor geautomatiseerde builds en GitHub Releases. Dit document beschrijft hoe het release proces werkt: van feature-complete tot productie.

---

## Overzicht

```
main
 │
 ├── feature/x  ──┐
 ├── feature/y  ──┤  normale ontwikkeling
 ├── feature/z  ──┘
 │
 │   [feature-complete voor 0.2.0]
 │
 ├── release/0.2.0
 │     ├─ tag v0.2.0-rc.1   → GitHub pre-release (automatisch)
 │     ├─ fix: ...
 │     ├─ tag v0.2.0-rc.2   → GitHub pre-release (automatisch)
 │     └─ tag v0.2.0        → GitHub release (automatisch)
 │
 └── main  ←── merge na release
```

Elke tag die je pusht triggert automatisch GoReleaser via GitHub Actions. Je hoeft verder niets handmatig te doen.

---

## Stap voor stap

### 1. Release branch aanmaken

Maak een release branch aan zodra de features voor deze versie feature-complete zijn. Nieuwe features gaan gewoon door op `main` — die komen in de volgende versie.

```bash
git checkout main
git pull
git checkout -b release/0.2.0
git push origin release/0.2.0
```

### 2. Release candidate taggen

Tag de eerste release candidate en push de tag. De GitHub Actions workflow start automatisch.

```bash
git tag v0.2.0-rc.1
git push origin v0.2.0-rc.1
```

GoReleaser herkent het `-rc` suffix en markeert de GitHub Release automatisch als **pre-release**.

### 3. Bugs fixen en nieuwe RC's taggen

Bugfixes worden gecommit op de release branch. Elke fix krijgt een nieuwe RC tag.

```bash
# Fix committen
git commit -m "fix: correct pagination in /Observation endpoint"

# Nieuwe RC
git tag v0.2.0-rc.2
git push origin v0.2.0-rc.2
```

### 4. Definitieve release

Zodra een RC goedgekeurd is (getest, gevalideerd), tag je de definitieve versie.

```bash
git tag v0.2.0
git push origin v0.2.0
```

GoReleaser maakt nu een volwaardige GitHub Release aan — geen pre-release.

### 5. Merge terug naar main

```bash
git checkout main
git merge release/0.2.0
git push origin main
```

De release branch kan daarna verwijderd worden.

---

## Wat GoReleaser automatisch doet

Bij elke tag (`v*`) triggert de pipeline en doet GoReleaser het volgende:

| Stap | Wat er gebeurt |
|---|---|
| **Build** | Compileert binaries voor linux, windows, darwin (amd64 + arm64) |
| **Versie injecten** | `main.version`, `main.commit`, `main.buildDate` worden ingebakken |
| **Archiveren** | Elke binary wordt verpakt met `README.md`, `LICENSE`, en `config/config.example.yaml` |
| **Checksums** | `checksums.txt` wordt gegenereerd voor alle artefacten |
| **Changelog** | Gegenereerd door GitHub's native release notes (`changelog.use: github-native`) |
| **GitHub Release** | Release aangemaakt met alle artefacten, changelog, en juiste pre-release vlag |

### Changelog

GoReleaser delegeert de changelog generatie aan GitHub's "auto-generated release notes": één regel per merged PR (inclusief auteur en PR-nummer), een contributors-lijst, en een "Full Changelog" compare-link. Losse commits binnen een merge commit worden niet los getoond — alleen de PR titel.

Zorg dus voor duidelijke PR-titels — die komen rechtstreeks in de release notes.

---

## Versienummering

FENIX volgt [Semantic Versioning](https://semver.org/):

```
MAJOR.MINOR.PATCH

0.2.0
│ │ └── bugfix, backwards compatible
│ └──── nieuwe feature, backwards compatible
└────── breaking change
```

Pre-release suffixes:

| Tag | Betekenis |
|---|---|
| `v0.2.0-rc.1` | Release candidate 1 — onder validatie |
| `v0.2.0-rc.2` | Tweede RC na bugfix |
| `v0.2.0` | Definitieve release |

We gebruiken geen `alpha` of `beta` tags — RC is voldoende voor onze workflow.

---

## Lokaal testen

Je kunt GoReleaser lokaal draaien zonder een echte release te maken:

**macOS:**
```bash
# Installeer GoReleaser
brew install goreleaser

# Dry-run — bouwt alles maar maakt geen GitHub Release
goreleaser release --snapshot --clean

# Output staat in ./dist/
ls dist/
```

**Windows (PowerShell):**
```powershell
# Installeer GoReleaser
choco install goreleaser

# Dry-run — bouwt alles maar maakt geen GitHub Release
goreleaser release --snapshot --clean

# Output staat in ./dist/
ls dist/
```

Handig om te controleren of de build slaagt voordat je een tag pusht.

---

## Veelgemaakte fouten

**Tag pushen op de verkeerde branch**
GoReleaser bouwt wat er op dat commit staat — zorg dat je op de release branch zit voordat je tagt.

```bash
# Check waar je bent
git log --oneline -5
git status
```

**`fetch-depth: 0` ontbreekt in de workflow**
GoReleaser heeft de volledige git history nodig voor de changelog. De workflow is al correct geconfigureerd, maar pas dit niet aan.

**Versie al bestaat**
Je kunt een tag niet opnieuw pushen. Als er iets mis ging: verwijder de tag lokaal en remote, fix het probleem, en push opnieuw.

```bash
git tag -d v0.2.0-rc.1
git push origin :refs/tags/v0.2.0-rc.1
```

---

## Configuratie

De volledige GoReleaser configuratie staat in `.goreleaser.yaml` in de root van de repo. Pas dit bestand aan als je:

- Nieuwe doelplatformen wilt toevoegen
- Extra bestanden wilt meeverpakken in het archief
- De changelog groepering wilt aanpassen

Wijzigingen aan `.goreleaser.yaml` kun je valideren met:

```bash
goreleaser check
```