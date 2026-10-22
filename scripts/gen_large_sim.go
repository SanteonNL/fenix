//go:build ignore

// Generates a large synthetic "sim" dataset (the same CSV layout as
// test/data/sim/) for load-testing Group $export — see config/large-export.yaml.
//
// Usage:
//
//	go run scripts/gen_large_sim.go [-patients 10000] [-obs 10] [-out output/large-export/data]
//
// Every patient gets -obs Observations; one in ten is status=preliminary, so
// Observation?status=final is a real subset. Output is deterministic, so two
// runs produce identical files.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	patients := flag.Int("patients", 10000, "number of patients")
	obsPer := flag.Int("obs", 10, "observations per patient")
	out := flag.String("out", filepath.Join("output", "large-export", "data"), "output directory")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}

	// Same column layout as test/data/sim/patient.csv, with the ";" delimiter
	// config/large-export.yaml declares for the whole source.
	pf, pw := create(filepath.Join(*out, "patient.csv"))
	fmt.Fprintln(pw, "Identificatienummer;GeslachtCode;GeslachtOmschrijving;Land;Geboortedatum;DatumOverlijden;DatumCheckStatusOverlijden")

	of, ow := create(filepath.Join(*out, "AlgemeneMeting.csv"))
	fmt.Fprintln(ow, "MetingID;Identificatienummer;MetingNaamCodeSysteem;MetingNaamCode;MetingNaamOmschrijving;UitslagWaarde;UitslagWaardeEenheidSysteem;UitslagWaardeEenheid;UitslagWaardeOperator;UitslagCodeSysteem;UitslagCode;UitslagCodeOmschrijving;UitslagDatumTijd;MetingDatumTijd;MeetmethodeCodeSysteem;MeetmethodeCode;MeetmethodeOmschrijving;InterpretatieVlaggenCodeSysteem;InterpretatieVlaggenCode;InterpretatieVlaggenOmschrijving;ReferentieWaardeBovengrens;ReferentieWaardeOndergrens;ReferentieWaardeEenheidSysteem;ReferentieWaardeEenheid;Status")

	codes := []string{"MCD_A", "MCD_L", "MCD_B", "MCD_P", "MCD_Q", "MCD_R", "MCD_S"}
	metingID := 0
	for p := 1; p <= *patients; p++ {
		id := fmt.Sprintf("%07d", p)
		gender, label := "M", "Male"
		if p%2 == 0 {
			gender, label = "F", "Female"
		}
		fmt.Fprintf(pw, "%s;%s;%s;NL;%d-%02d-15;;\n", id, gender, label, 1940+p%60, 1+p%12)

		for o := 0; o < *obsPer; o++ {
			metingID++
			status := "final"
			if o%10 == 9 {
				status = "preliminary"
			}
			code := codes[o%len(codes)]
			ts := fmt.Sprintf("2023-%02d-%02dT%02d:%02d:00+02:00", 1+o%12, 1+o%28, o%24, o%60)
			if o%2 == 0 { // numeric result
				fmt.Fprintf(ow, "%d;%s;https://santeon.nl/sct;%s;;%d;http://unitsofmeasure.org;bpm;;;;;;%s;;;;;;;;;;;%s\n",
					metingID, id, code, 60+o%40, ts, status)
			} else { // coded result
				fmt.Fprintf(ow, "%d;%s;https://santeon.nl/sct;%s;;;;;;https://santeon.nl/sct;A2;;;%s;;;;;;;;;;;%s\n",
					metingID, id, code, ts, status)
			}
		}
	}

	for _, f := range []struct {
		w *bufio.Writer
		f *os.File
	}{{pw, pf}, {ow, of}} {
		if err := f.w.Flush(); err != nil {
			fatal(err)
		}
		if err := f.f.Close(); err != nil {
			fatal(err)
		}
	}
	fmt.Printf("wrote %d patients and %d observations to %s\n", *patients, metingID, *out)
}

func create(path string) (*os.File, *bufio.Writer) {
	f, err := os.Create(path)
	if err != nil {
		fatal(err)
	}
	return f, bufio.NewWriterSize(f, 1<<20)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
