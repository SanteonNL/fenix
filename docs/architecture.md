FENIX Architecture
Status: Draft
Last updated: 2025-01-XX

Authors: Tommy, [team]
1. Overview
FENIX (FHIR Enabled Node for Information Exchange) is a data transformation platform that converts healthcare data from various source systems into standardized FHIR R4 format for exchange between Dutch healthcare organizations.
1.1 Problem Statement
Dutch healthcare organizations struggle with data availability and standardization. Each hospital has different EPD systems, custom data formats, and siloed implementations. This makes quality registrations, research, and AI innovation difficult.
1.2 Goals

Transform data from multiple EPD systems (Epic, HiX, etc.) to FHIR R4
Support national standards (nl-core profiles, Nictiz specifications)
Be usable by any Dutch healthcare organization, not just Santeon
Open source and vendor-neutral

1.3 Non-Goals

Real-time clinical decision support (batch processing focus)
Replacing EPD systems
Building a data warehouse (FENIX transforms, doesn't store long-term)


6. Deployment
6.1 Design Principle
FENIX prioritizes minimal deployment complexity. A hospital IT department should be able to run FENIX by downloading a single executable — no runtime dependencies, no complex installation procedures.
6.2 Runtime Environment

Single .exe binary — no Java, no Python, no runtime installations required
Configuration via environment variables or single config file
Works on Windows Server (primary target) and Linux

6.3 Self-Updating Executable
The FENIX executable can automatically download and update itself from the central Santeon DLS storage account.
┌─────────────────────────────────────────────────────────────────┐
│                     Auto-Update Flow                            │
│                                                                 │
│   Hospital                              Central (Santeon)       │
│                                                                 │
│   ┌──────────┐     "version:latest"     ┌──────────────────┐   │
│   │  fenix   │ ──────────────`─────────▶│ dls-hips-p       │   │
│   │  .exe    │                          │ /releases/       │   │
│   └────┬─────┘     downloads newest     │   fenix-1.2.0.exe│   │
│        │        ◀───────────────────────│   fenix-1.1.0.exe│   │
│        ▼                                │   fenix-1.0.0.exe│   │
│   ┌──────────┐                          │   latest.txt     │   │
│   │  fenix   │                          └──────────────────┘   │
│   │ (updated)│                                                 │
│   └──────────┘                                                 │
└─────────────────────────────────────────────────────────────────┘
Version modes:
CommandBehaviorfenix.exe --version:latestDownloads and runs the latest releasefenix.exe --version:1.2.0Downloads and runs specific versionfenix.exeRuns current local version (no update check)
Storage locations:

Production: dls-hips-p.santeon.nl/releases/fenix/
Test: dls-hips-t.santeon.nl/releases/fenix/

6.4 Database Options
FENIX uses a temporary database for intermediate processing during transformations. Three deployment modes are supported, in order of performance:
ModePerformanceSetup ComplexityUse CaseExternal databaseBestHospital provides/configuresProduction, large datasetsDocker containerGoodFENIX spins up containerProduction, simpler setupIn-memory (embedded)SlowerZero setupDevelopment, small datasets, demos
┌─────────────────────────────────────────────────────────────────┐
│                     Database Options                            │
│                                                                 │
│  Option A: External DB          Option B: Docker      Option C: In-Memory
│  (hospital-managed)             (auto-provisioned)    (embedded)
│                                                                 │
│  ┌──────────┐                   ┌──────────┐          ┌──────────┐
│  │  fenix   │                   │  fenix   │          │  fenix   │
│  │  .exe    │                   │  .exe    │          │  .exe    │
│  └────┬─────┘                   └────┬─────┘          └──────────┘
│       │                              │                     │
│       ▼                              ▼                     ▼
│  ┌──────────┐                   ┌──────────┐          ┌──────────┐
│  │ Hospital │                   │  Docker  │          │ In-memory│
│  │ Postgres │                   │ Postgres │          │   SQLite │
│  │ /SQL Srv │                   │ container│          │          │
│  └──────────┘                   └──────────┘          └──────────┘
│                                                                 │
│  Best performance               Good performance      Simplest setup
│  Hospital maintains DB          Requires Docker       No dependencies
└─────────────────────────────────────────────────────────────────┘
6.5 Centralized Logging
FENIX can send logs to a central Azure Log Analytics workspace for monitoring across all hospital deployments.
┌─────────────────────────────────────────────────────────────────┐
│                     Logging Architecture                        │
│                                                                 │
│  Hospital A          Hospital B          Hospital C             │
│  ┌────────┐          ┌────────┐          ┌────────┐            │
│  │ fenix  │          │ fenix  │          │ fenix  │            │
│  └───┬────┘          └───┬────┘          └───┬────┘            │
│      │                   │                   │                  │
│      └───────────────────┼───────────────────┘                  │
│                          │                                      │
│                          ▼                                      │
│              ┌───────────────────────┐                         │
│              │  Azure Log Analytics  │                         │
│              │  (Central Workspace)  │                         │
│              └───────────────────────┘                         │
│                          │                                      │
│                          ▼                                      │
│              ┌───────────────────────┐                         │
│              │  Dashboards / Alerts  │                         │
│              │  - Transformation runs│                         │
│              │  - Errors per hospital│                         │
│              │  - Performance metrics│                         │
│              └───────────────────────┘                         │
└─────────────────────────────────────────────────────────────────┘
Logging modes:
ModeDescriptionlocalLogs to file/stdout only (default, no network needed)azureLogs to Azure Log Analytics + localbothFull redundancy
What gets logged centrally:

Transformation job start/end, duration, record counts
Errors and warnings
Version information
No patient data (PHI) — only operational metrics

6.6 Configuration Example
yaml# fenix.yaml
version:
  check: "latest"  # latest | 1.2.0 | none
  storage_url: "https://dls-hips-p.santeon.nl/releases/fenix"

database:
  mode: "external"  # external | docker | memory
  
  # Only needed for mode: external
  connection_string: "postgres://user:pass@hospital-db:5432/fenix"
  
  # Only needed for mode: docker
  docker_image: "postgres:15-alpine"
  docker_port: 5433

logging:
  mode: "azure"  # local | azure | both
  level: "info"  # debug | info | warn | error
  
  # Only needed for mode: azure or both
  azure:
    workspace_id: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    shared_key: "${AZURE_LOG_KEY}"  # from environment variable
    
  # Local file logging
  local:
    path: "./logs/fenix.log"
    max_size_mb: 100
    max_files: 5

input:
  path: "./input"
  
output:
  path: "./output"
  
mappings:
  source_system: "epic"  # epic | hix | ...
6.7 Per-Hospital Deployment
Each hospital runs its own FENIX instance locally. Patient data never leaves the hospital network — only operational logs (no PHI) are sent centrally.
Hospital A (Epic)                 Hospital B (HiX)
┌─────────────────────┐          ┌─────────────────────┐
│  fenix.exe          │          │  fenix.exe          │
│  fenix.yaml         │          │  fenix.yaml         │
│   └─ source: epic   │          │   └─ source: hix    │
│  /input             │          │  /input             │
│  /output            │          │  /output            │
│  /logs              │          │  /logs              │
└─────────────────────┘          └─────────────────────┘
         │                                │
         └────────── logs only ───────────┘
                      │
                      ▼
              Azure Log Analytics