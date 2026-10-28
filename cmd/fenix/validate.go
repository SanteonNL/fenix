package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/SanteonNL/fenix/internal/config"
	"github.com/SanteonNL/fenix/internal/querycompiler"
	"github.com/SanteonNL/fenix/internal/queryvalidator"
	"github.com/rs/zerolog"
)

var (
	validateMinSeverity = flag.String("min-severity", "info", "Lowest severity shown by -cmd validate: info | warning | error")
	validateJSON        = flag.String("validate-json", "", "Also write the validation report as JSON to this file (-cmd validate)")
)

// runValidate statically validates the conversion SQL files against the FHIR
// model and the configured profiles. It never opens the database. It returns
// false when any error-level finding exists.
func runValidate(cfg *config.Config, repoRoot string, sourceNames []string, log *zerolog.Logger) bool {
	min, ok := queryvalidator.ParseSeverity(*validateMinSeverity)
	if !ok {
		log.Fatal().Str("min-severity", *validateMinSeverity).Msg("Unknown severity, use info, warning or error")
	}

	files := validationFiles(cfg, repoRoot, sourceNames, log)
	if len(files) == 0 {
		log.Warn().Msg("No SQL files found to validate")
		return true
	}

	profileSvc, _, _, _ := loadTerminology(cfg, repoRoot, log)
	v := &queryvalidator.Validator{Profile: profileSvc}

	var reports []*queryvalidator.FileReport
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			log.Error().Err(err).Str("file", f).Msg("Failed to read SQL file")
			continue
		}
		// Same rendering as a batch run: pushdown conditionals evaluate to "absent".
		sql, err := querycompiler.RenderSQL(string(raw), nil)
		if err != nil {
			log.Error().Err(err).Str("file", f).Msg("Failed to render SQL template")
			continue
		}
		name := f
		if rel, err := filepath.Rel(repoRoot, f); err == nil {
			name = rel
		}
		reports = append(reports, v.ValidateSQL(name, sql))
	}

	errs, warns := queryvalidator.WriteText(os.Stdout, reports, min)
	fmt.Printf("%d file(s): %d error(s), %d warning(s)\n", len(reports), errs, warns)

	if *validateJSON != "" {
		out, err := os.Create(*validateJSON)
		if err == nil {
			err = queryvalidator.WriteJSON(out, reports)
			out.Close()
		}
		if err != nil {
			log.Error().Err(err).Str("file", *validateJSON).Msg("Failed to write JSON report")
		}
	}
	return errs == 0
}

// validationFiles lists the same SQL files a batch conversion would run: the
// -sql / inputs.sqlFile file, plus queries/<source>/fhir/*.sql per source.
func validationFiles(cfg *config.Config, repoRoot string, sourceNames []string, log *zerolog.Logger) []string {
	if *sqlFile != "" {
		return []string{resolvePath(repoRoot, *sqlFile)}
	}
	var files []string
	if cfg.Inputs.SQLFile != "" {
		files = append(files, resolvePath(repoRoot, cfg.Inputs.SQLFile))
	}
	root := cfg.QueriesRoot(context.Background(), repoRoot, *log)
	var names []string
	for name := range cfg.Sources {
		if len(sourceNames) == 0 || contains(sourceNames, name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		dir := resolvePath(root, "queries/"+name+"/fhir")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
				files = append(files, filepath.Join(dir, e.Name()))
			}
		}
	}
	return files
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
