package main

import (
	"context"
	"strings"

	"github.com/SanteonNL/fenix/internal/config"
	"github.com/SanteonNL/fenix/internal/conformance"
	"github.com/SanteonNL/fenix/internal/converter"
	"github.com/SanteonNL/fenix/internal/datasets"
	"github.com/SanteonNL/fenix/internal/fhirpackage"
	"github.com/rs/zerolog"
)

// loadConformance builds the conformance store the FHIR API serves and the
// :in / :not-in modifiers use:
//   - every resource in the terminology input folders (dirs),
//   - every resource of the inputs.packages (and their dependencies),
//   - the inputs.terminologyServers, for what cannot be expanded locally.
//
// A package's own resources are also fed into conversion for each entry in its
// "use" (see config.PackageRef). obsDefs is returned, created when a package
// brings ObservationDefinitions and none were loaded from a folder.
func loadConformance(ctx context.Context, cfg *config.Config, repoRoot string, log *zerolog.Logger, dirs []string,
	profileSvc *converter.ProfileService, conceptMapSvc *converter.ConceptMapService, obsDefs *converter.ObservationDefinitions,
	ds *datasets.Catalog,
) (*conformance.Store, *converter.ObservationDefinitions) {
	store := conformance.NewStore(*log)
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if _, err := store.AddDir(dir); err != nil {
			log.Warn().Err(err).Str("dir", dir).Msg("Failed to read conformance resources")
		}
	}

	var refs []fhirpackage.Ref
	for _, p := range cfg.Inputs.Packages {
		if err := p.Validate(); err != nil {
			log.Error().Err(err).Msg("Skipping package")
			continue
		}
		ref := fhirpackage.Ref{Name: p.Name, Version: p.Version, Registry: p.Registry, TokenEnv: p.TokenEnv,
			Follow: p.Dependencies, Exclude: p.Exclude, Use: p.Use}
		if p.File != nil {
			path, err := config.ResolveDir(ctx, cfg, *p.File, repoRoot, *log)
			if err != nil {
				log.Error().Err(err).Str("file", p.File.Path).Msg("Skipping package")
				continue
			}
			ref.File = path
		}
		refs = append(refs, ref)
	}
	if len(refs) > 0 {
		loader := fhirpackage.NewLoader(cfg.Inputs.PackageCacheDir(repoRoot), *log)
		pkgs, err := loader.Load(ctx, refs)
		if err != nil {
			log.Error().Err(err).Msg("Failed to load FHIR package(s)")
		}
		for _, pkg := range pkgs {
			obsDefs = usePackage(pkg, store, profileSvc, conceptMapSvc, obsDefs, ds, log)
		}
	}

	for _, ts := range cfg.Inputs.TerminologyServers {
		if err := ts.Validate(); err != nil {
			log.Error().Err(err).Msg("Skipping terminology server")
			continue
		}
		ttl, _ := ts.TTL()
		store.AddRemote(&conformance.RemoteServer{
			Name: ts.Name, BaseURL: ts.URL, ValueSets: ts.ValueSets, Systems: ts.Systems,
			CacheTTL: ttl, MaxSize: ts.MaxConcepts,
			Auth: conformance.Auth{Type: ts.Auth.Type, TokenEnv: ts.Auth.TokenEnv, TokenURL: ts.Auth.TokenURL,
				ClientID: ts.Auth.ClientID, UsernameEnv: ts.Auth.UsernameEnv, PasswordEnv: ts.Auth.PasswordEnv},
		})
		log.Info().Str("server", ts.Name).Str("url", ts.URL).Strs("systems", ts.Systems).Strs("valueSets", ts.ValueSets).
			Msg("Terminology server configured")
	}

	log.Info().Any("resources", store.Count()).Msg("Conformance resources loaded")
	return store, obsDefs
}

// usePackage adds every resource of a package to the store and feeds its own
// resources into conversion as its "use" says.
func usePackage(pkg fhirpackage.Package, store *conformance.Store, profileSvc *converter.ProfileService,
	conceptMapSvc *converter.ConceptMapService, obsDefs *converter.ObservationDefinitions, ds *datasets.Catalog, log *zerolog.Logger,
) *converter.ObservationDefinitions {
	byType, err := pkg.Resources()
	if err != nil {
		log.Error().Err(err).Str("package", pkg.Name).Msg("Failed to read FHIR package")
		return obsDefs
	}
	served := 0
	for _, files := range byType {
		served += store.AddFiles(files)
	}
	ev := log.Info().Str("package", pkg.Name+"#"+pkg.Version).Bool("dependency", pkg.Dependency).Int("served", served)
	for _, use := range pkg.Use {
		switch use {
		case "profiles":
			n := 0
			for _, f := range byType["StructureDefinition"] {
				if err := profileSvc.LoadProfile(f); err != nil {
					log.Warn().Err(err).Str("file", f).Msg("Skipping profile")
					continue
				}
				n++
			}
			ev = ev.Int("profiles", n)
		case "valueSets":
			ev = ev.Int("valueSets", conceptMapSvc.AddValueSetFiles(byType["ValueSet"]))
		case "conceptMaps":
			ev = ev.Int("conceptMaps", conceptMapSvc.AddConceptMapFiles(byType["ConceptMap"]))
		case "observationDefinitions":
			if obsDefs == nil {
				obsDefs = converter.NewObservationDefinitions(conceptMapSvc, *log)
			}
			ev = ev.Int("observationDefinitions", obsDefs.AddFiles(byType["ObservationDefinition"]))
		case "datasets":
			n, err := ds.AddFiles(byType["Binary"])
			if err != nil {
				log.Warn().Err(err).Str("package", pkg.Name).Msg("Skipping unreadable dataset definitions")
			}
			ev = ev.Int("datasetDefinitions", n).Strs("datasets", ds.IDs())
		}
	}
	if len(pkg.Use) > 0 {
		ev = ev.Str("use", strings.Join(pkg.Use, ","))
	}
	ev.Msg("Loaded FHIR package")
	return obsDefs
}
