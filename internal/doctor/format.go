package doctor

import (
	"fmt"
	"io"
	"strings"
)

// FormatReport writes a human-readable doctor report to w.
// When findings is empty, writes a single ok line.
func FormatReport(w io.Writer, findings []Finding) error {
	if len(findings) == 0 {
		_, err := fmt.Fprintln(w, "ok: no issues found")
		return err
	}
	for i, f := range findings {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		header := fmt.Sprintf("[%s]", f.Kind)
		if f.Repo != "" {
			header += " " + f.Repo
		}
		if _, err := fmt.Fprintf(w, "%s: %s\n", header, f.Message); err != nil {
			return err
		}
		if f.Branch != "" {
			if _, err := fmt.Fprintf(w, "  branch: %s\n", f.Branch); err != nil {
				return err
			}
		}
		if len(f.Suggest) > 0 {
			if _, err := fmt.Fprintln(w, "  suggest:"); err != nil {
				return err
			}
			for _, s := range f.Suggest {
				line := s
				if strings.HasPrefix(line, "#") {
					if _, err := fmt.Fprintf(w, "    %s\n", line); err != nil {
						return err
					}
					continue
				}
				if _, err := fmt.Fprintf(w, "    %s\n", line); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
