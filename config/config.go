package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// LET OP: de commentaren bij de config-structs en hun velden hieronder worden de hovertekst in
// de editor (via fenix-config.schema.json). Pas je een struct, een veld, een tag OF alleen een
// beschrijving aan? Draai dan `make schema` en commit het bijgewerkte schema in dezelfde commit.
//
// De beschrijvingen zijn in het Nederlands, omdat ze voor de (Nederlandse) gebruikers van het
// configbestand zijn. Elke beschrijving begint met "Verplicht" of "Optioneel"; in het schema is
// alleen een veld met de tag `jsonschema:"required"` verplicht (de rest is standaard optioneel).
// Welke bronvelden per `type` verplicht zijn, staat in tools/genschema (fieldsBySourceType).
//
//go:generate go -C ../tools run ./genschema -root .. -out ../fenix-config.schema.json

// Config is het hoofdniveau van het Fenix-configbestand. Relatieve paden worden gelezen
// vanaf de hoofdmap van de Fenix-repository.
type Config struct {
	// Optioneel. Vorm van de logregels. `dev`: gekleurd en leesbaar in de console. `prod`: één JSON-object per regel.
	Environment string `yaml:"environment" jsonschema:"enum=dev,enum=prod"`

	// Optioneel. Laagste logniveau dat getoond wordt. Standaard: `debug` bij dev, `info` bij prod.
	LogLevel string `yaml:"logLevel" jsonschema:"enum=trace,enum=debug,enum=info,enum=warn,enum=error"`

	// Optioneel. De database waarin brondata wordt geladen vóór de conversie (de stagingdatabase).
	// Standaard: een SQLite-database in het geheugen.
	Staging StagingConfig `yaml:"staging"`

	// Optioneel. Hoe de stagingdata naar FHIR wordt omgezet.
	FHIR FHIRConfig `yaml:"fhir"`

	// Optioneel. Waar en in welk formaat de FHIR-uitvoer wordt geschreven.
	Output OutputConfig `yaml:"output"`

	// Optioneel. De bronnen die in staging worden geladen, elk onder een zelfgekozen naam.
	// Bij `local` en `sftp` wordt die naam het begin van elke stagingtabel.
	//
	// Voorbeeld: een bron `pja` die `journeys.json` leest, maakt de tabel `pja_journeys`.
	Sources SourcesConfig `yaml:"sources"`
}

// SourcesConfig maps source name (e.g. "luscii") to its configuration.
type SourcesConfig map[string]SourceConfig

// EndpointConfig is één Luscii-API-endpoint dat in een eigen stagingtabel wordt geladen.
type EndpointConfig struct {
	// Verplicht. URL-pad van het endpoint, achter `base_url`. Voorbeeld: `/v1/patients`.
	Path string `yaml:"path" jsonschema:"required"`

	// Verplicht. Stagingtabel waarin de records komen. Voorbeeld: `luscii_patients`.
	Table string `yaml:"table" jsonschema:"required"`

	// Optioneel. Queryparameter voor "gewijzigd sinds". Als deze is ingevuld, haalt een volgende run
	// alleen records op die sinds de vorige run zijn gewijzigd (incrementeel laden). Voorbeeld: `startDate`.
	SinceParam string `yaml:"since_param"`

	// Optioneel. Queryparameter voor het einde van de periode; wordt samen met `since_param` meegestuurd.
	// Voorbeeld: `endDate`.
	EndParam string `yaml:"end_param"`

	// Optioneel, verouderd: gebruik `id_field` onder `json_options.<table>`.
	// Veld dat uniek is per record; wordt gebruikt om rijen bij te werken bij incrementeel laden.
	IDField string `yaml:"id_field"`
}

// SourceConfig is één bron. Welke velden gelden, hangt af van `type`; het schema
// markeert velden die niet bij het gekozen type horen.
type SourceConfig struct {
	// Verplicht. Soort bron; elk type gebruikt zijn eigen velden.
	//
	// - `local`: leest CSV- en JSON-bestanden uit een map. Verplicht: `dir`.
	// - `sftp`: downloadt CSV- en JSON-bestanden van een SFTP-server en leest ze daarna als `local`. Verplicht: `host`, `username`, `key_file`.
	// - `sqldb`: voert SQL-queries uit op een externe database. Verplicht: `connection_string`.
	// - `luscii`: haalt records op via de Luscii-REST-API. Verplicht: `base_url`, `api_key`, `endpoints`.
	//
	// Een type werkt alleen als het programma dat Fenix draait dat type bevat; anders wordt de bron
	// overgeslagen met "unknown source type" in de log.
	Type string `yaml:"type" jsonschema:"required,enum=local,enum=sftp,enum=sqldb,enum=luscii"`

	// Verplicht bij `luscii`. Basis-URL van de API. Voorbeeld: `https://vitalsapi.luscii.com`.
	BaseURL string `yaml:"base_url"`

	// Verplicht bij `luscii`. API-sleutel, meegestuurd als Bearer-token. Zet hier een omgevingsvariabele,
	// niet de sleutel zelf. Voorbeeld: `${LUSCII_API_KEY}`.
	APIKey string `yaml:"api_key"`

	// Verplicht bij `local`. Map met de `.csv`- en `.json`-bestanden die geladen worden. Elk bestand
	// wordt een tabel `<bron>_<bestandsnaam>`. Voorbeeld: `test/data/sim`.
	Dir string `yaml:"dir"`

	// Optioneel, bij `local` en `sftp`. Scheidingsteken tussen kolommen in CSV-bestanden (één teken).
	// Standaard: `,`. Voorbeeld: `";"`.
	Delimiter string `yaml:"delimiter"`

	// Verplicht bij `sqldb`. Connectionstring van de database. Zet inloggegevens in omgevingsvariabelen.
	// Voorbeeld: `server=127.0.0.1;port=1433;user id=${HIX_USER};password=${HIX_PASSWORD};database=hix`.
	ConnectionString string `yaml:"connection_string"`

	// Optioneel, bij `sqldb`. Map met de `.sql`-bestanden die worden uitgevoerd; elk queryresultaat
	// wordt een stagingtabel. Standaard: `queries/<bron>/staging`.
	StagingDir string `yaml:"staging_dir"`

	// Verplicht bij `sftp`. Hostnaam of IP-adres van de SFTP-server. Voorbeeld: `${ZBJ_SFTP_HOST}`.
	Host string `yaml:"host"`

	// Optioneel, bij `sftp`. Poort van de SFTP-server. Standaard: `22`.
	Port int `yaml:"port"`

	// Verplicht bij `sftp`. Gebruikersnaam om mee in te loggen. Voorbeeld: `${ZBJ_SFTP_USER}`.
	Username string `yaml:"username"`

	// Verplicht bij `sftp`. Pad naar het bestand met de private SSH-sleutel om mee in te loggen.
	// Voorbeeld: `C:\\Users\\<gebruiker>\\.ssh\\id_rsa`.
	KeyFile string `yaml:"key_file"`

	// Optioneel, bij `sftp`. Map op de server waaruit bestanden worden gedownload. Standaard: `/`. Voorbeeld: `/pja`.
	RemoteDir string `yaml:"remote_dir"`

	// Verplicht bij `luscii`. De API-endpoints die worden opgehaald; elk vult een eigen stagingtabel.
	Endpoints []EndpointConfig `yaml:"endpoints"`

	// Optioneel, bij `local`, `sftp` en `luscii`. Hoe JSON-records over tabellen worden verdeeld.
	// (De beschrijving in het schema staat in tools/genschema, omdat dit veld nog geen vaste structuur heeft.)
	JSONOptions map[string]interface{} `yaml:"json_options"`
}

// EffectiveLogLevel returns the log level to use, applying the smart default:
// dev → debug, prod → info (unless explicitly overridden).
func (c *Config) EffectiveLogLevel() string {
	if c.LogLevel != "" {
		return c.LogLevel
	}
	if c.Environment == "dev" {
		return "debug"
	}
	return "info"
}

// StagingConfig is de database waarin brondata wordt geladen vóór de conversie.
type StagingConfig struct {
	// Optioneel. Soort database. Standaard: `sqlite`.
	Database string `yaml:"database" jsonschema:"enum=sqlite,enum=postgres,enum=sqlserver"`

	// Optioneel, bij `sqlite`. Driver. `sqlite` (standaard) is puur Go; `sqlite3` heeft CGO en een C-compiler nodig.
	Driver string `yaml:"driver" jsonschema:"enum=sqlite,enum=sqlite3"`

	// Optioneel, bij `sqlite`. Databasebestand. Laat weg voor een database in het geheugen, die na de run weg is.
	// Een bestand is handig om de stagingtabellen achteraf te bekijken. Voorbeeld: `output/staging.db`.
	Path string `yaml:"path"`

	// Verplicht bij `postgres` en `sqlserver`. Connectionstring. Voorbeeld: `host=localhost user=fenix dbname=staging`.
	Connection string `yaml:"connection"`

	// Optioneel. Schrijf de stagingdata ook naar bestanden (een platte CSV per tabel, plus hiërarchische JSON).
	Files *StagingFilesConfig `yaml:"files,omitempty"`
}

// StagingFilesConfig schrijft de stagingdata naar platte CSV-bestanden en hiërarchische JSON-bestanden,
// naast de stagingdatabase. Bij incrementeel laden worden nieuwe CSV-rijen toegevoegd en JSON-records
// samengevoegd op hun id.
type StagingFilesConfig struct {
	// Verplicht. Map voor de platte CSV-bestanden, één bestand per tabel. Voorbeeld: `output/staging`.
	Dir string `yaml:"dir" jsonschema:"required"`

	// Optioneel. Map voor de hiërarchische JSON-bestanden. Standaard: dezelfde map als `dir`.
	RawDir string `yaml:"raw_dir"`
	// Future: Azure blob storage support
	// Azure *AzureStorageConfig `yaml:"azure,omitempty"`
}

// StagingPath returns the SQLite path to use.
// Empty string means in-memory (":memory:"), which is the default.
func (sc *StagingConfig) StagingPath() string {
	if sc.Path == "" {
		return ":memory:"
	}
	return sc.Path
}

// SQLiteDriver returns the configured driver name, defaulting to "sqlite" (pure Go, no CGO).
func (sc *StagingConfig) SQLiteDriver() string {
	if sc.Driver != "" {
		return sc.Driver
	}
	return "sqlite"
}

// FHIRConfig is hoe de stagingdata naar FHIR-resources wordt omgezet.
type FHIRConfig struct {
	// Optioneel. SQL-bestand dat de stagingtabellen naar FHIR omzet; mag meerdere statements bevatten.
	// Laat weg om de algemene conversie over te slaan. De opdrachtregeloptie `-sql` gaat hiervoor.
	SQLFile string `yaml:"sqlFile"`

	// Optioneel, nog niet in gebruik. Naam of repository van het FHIR-profiel. Voorbeeld: `sim-on-fhir`.
	Profile string `yaml:"profile"`

	// Optioneel. Map met FHIR-StructureDefinitions als `.json`-bestanden. Voorbeeld: `terminology/profiles/sim`.
	ProfilesDir string `yaml:"profilesDir"`

	// Optioneel. Map met conceptmaps als platte CSV-bestanden. Voorbeeld: `terminology/conceptmaps/flat`.
	ConceptMapsDir string `yaml:"conceptMapsDir"`
}

// OutputConfig is waar en in welk formaat de FHIR-uitvoer wordt geschreven.
type OutputConfig struct {
	// Optioneel. Bestandsformaat. `json`: één JSON-array. `ndjson`: één resource per regel.
	// `pretty`: ingesprongen JSON. Standaard: `json`.
	Format string `yaml:"format" jsonschema:"enum=json,enum=ndjson,enum=pretty"`

	// Optioneel. Bestemming. Standaard: `local`. `datalake` is nog niet in gebruik.
	Type string `yaml:"type" jsonschema:"enum=local,enum=datalake"`

	// Optioneel. Instellingen voor `type: local`.
	Local LocalOutputConfig `yaml:"local"`

	// Optioneel, nog niet in gebruik. Instellingen voor `type: datalake`.
	DataLake *DataLakeConfig `yaml:"datalake,omitempty"`
}

// LocalOutputConfig is uitvoer naar een lokale map.
type LocalOutputConfig struct {
	// Optioneel. Uitvoermap. Standaard: `output`.
	Dir string `yaml:"dir"`

	// Optioneel. Schrijf elke run naar een eigen submap met datum en tijd. Standaard: `true`.
	UseTimestamp bool `yaml:"useTimestamp"`

	// Optioneel. Aantal eerdere runs dat in de archiefmap blijft; `0` bewaart alles. Standaard: `5`.
	// Geldt alleen als `useTimestamp` aan staat.
	ArchiveCount int `yaml:"archiveCount"`
}

// DataLakeConfig is de koppeling met een datagateway (bv. dls-t.hips.santeon.nl). Nog niet in gebruik.
type DataLakeConfig struct {
	URL            string `yaml:"url"`
	Path           string `yaml:"path"`
	ClientID       string `yaml:"clientId"`
	Certificate    string `yaml:"certificate"`
	CertificateKey string `yaml:"certificateKey"`
}

// LoadConfig loads configuration from a YAML file, resolving environment variables.
// Use ${ENV_VAR_NAME} in the YAML to reference environment variables.
// A .env file co-located with the config file is loaded automatically.
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	_ = godotenv.Load(filepath.Join(filepath.Dir(filePath), ".env"))

	if err := validateEnvVars(data); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	resolvedData, err := resolveEnvVars(data)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve environment variables: %w", err)
	}

	cfg := &Config{
		Staging: StagingConfig{Database: "sqlite"},
		Output: OutputConfig{
			Format: "json",
			Type:   "local",
			Local:  LocalOutputConfig{Dir: "output", UseTimestamp: true, ArchiveCount: 5},
		},
	}

	if err := yaml.Unmarshal(resolvedData, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	return cfg, nil
}

// NewStagingDB opens a staging database connection from the given config.
// The caller is responsible for blank-importing the required driver:
//
//	SQLite:     _ "modernc.org/sqlite"
//	PostgreSQL: _ "github.com/lib/pq"
//	SQL Server: _ "github.com/microsoft/go-mssqldb"
func NewStagingDB(cfg *Config) (*sqlx.DB, error) {
	switch cfg.Staging.Database {
	case "", "sqlite":
		dbPath := cfg.Staging.StagingPath()
		if dbPath != ":memory:" {
			if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return nil, fmt.Errorf("create staging dir: %w", err)
				}
			}
		}
		return sqlx.Connect(cfg.Staging.SQLiteDriver(), dbPath)
	case "postgres":
		return sqlx.Connect("postgres", cfg.Staging.Connection)
	case "sqlserver":
		return sqlx.Connect("sqlserver", cfg.Staging.Connection)
	default:
		return nil, fmt.Errorf("unsupported staging database: %s", cfg.Staging.Database)
	}
}

func resolveEnvVars(data []byte) ([]byte, error) {
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	result := re.ReplaceAllStringFunc(string(data), func(match string) string {
		varName := match[2 : len(match)-1]
		if value, exists := os.LookupEnv(varName); exists {
			return value
		}
		return match
	})
	return []byte(result), nil
}

func validateEnvVars(data []byte) error {
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	var missing []string
	for _, line := range strings.Split(string(data), "\n") {
		for _, match := range re.FindAllStringSubmatch(stripYAMLComment(line), -1) {
			if _, exists := os.LookupEnv(match[1]); !exists {
				missing = append(missing, match[1])
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

// stripYAMLComment returns the line with any trailing "# ..." comment removed,
// ignoring '#' characters that appear inside single- or double-quoted strings.
func stripYAMLComment(line string) string {
	inSingle, inDouble := false, false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}
