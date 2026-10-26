package queryvalidator

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// WriteText prints reports for humans, listing only findings at or above min.
// It returns the number of errors and warnings it saw (regardless of min).
func WriteText(w io.Writer, reports []*FileReport, min Severity) (errors, warnings int) {
	for _, r := range reports {
		errors += r.Count(Error)
		warnings += r.Count(Warning)
		shown := 0
		for _, f := range r.Findings {
			if f.Severity >= min {
				shown++
			}
		}
		status := "ok"
		if shown > 0 {
			status = fmt.Sprintf("%d error(s), %d warning(s), %d info", r.Count(Error), r.Count(Warning), r.Count(Info))
		}
		fmt.Fprintf(w, "%s  (%d statements)  %s\n", r.File, r.Statements, status)
		for _, f := range r.Findings {
			if f.Severity < min {
				continue
			}
			where := f.Path
			if f.Column != "" {
				where += " :: " + f.Column
			}
			if f.Statement > 0 {
				where += fmt.Sprintf("  [statement %d]", f.Statement)
			}
			fmt.Fprintf(w, "  [%-5s] %s  %s\n", f.Severity, f.Code, strings.TrimSpace(where))
			fmt.Fprintf(w, "          %s\n", f.Message)
			for _, e := range f.Examples {
				fmt.Fprintf(w, "          e.g. %s\n", e)
			}
		}
		fmt.Fprintln(w)
	}
	return errors, warnings
}

// WriteJSON writes the reports as indented JSON.
func WriteJSON(w io.Writer, reports []*FileReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(reports)
}
