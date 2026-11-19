package hive

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/grokify/oscompat/fs"
	"github.com/grokify/oscompat/paths"
)

// AppName names the per-user directories PlexusHive uses.
const AppName = "plexushive"

// ReportsEnv overrides the directory reports are written to and read from.
const ReportsEnv = "HIVE_REPORTS"

// ReportsDir returns the directory for status reports: $HIVE_REPORTS when
// set, otherwise "reports" under the platform's per-user data directory,
// which is created if needed. Reports describe one machine's sessions, so
// they are kept out of the hive repository.
func ReportsDir() (string, error) {
	if dir := os.Getenv(ReportsEnv); dir != "" {
		return dir, nil
	}
	base, err := paths.AppData(AppName)
	if err != nil {
		return "", fmt.Errorf("hive: reports directory: %w", err)
	}
	return filepath.Join(base, "reports"), nil
}

// WriteReport validates r and writes it to dir as <role>.json, replacing
// the role's previous report. The write is atomic: a reader sees the old
// report or the new one, never part of either.
func WriteReport(dir string, r Report) error {
	if err := r.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("hive: %w", err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("hive: %w", err)
	}
	if err := fs.WriteFileAtomic(filepath.Join(dir, r.Role+".json"), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("hive: %w", err)
	}
	return nil
}

// LoadReports reads every <role>.json in dir. A directory that does not
// exist yet holds no reports.
func LoadReports(dir string) ([]Report, error) {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	var out []Report
	err := eachJSON(dir, func(stem string, data []byte) error {
		var r Report
		if err := decodeStrict(data, &r); err != nil {
			return err
		}
		if r.Role != stem {
			return fmt.Errorf("role %q does not match the file name", r.Role)
		}
		out = append(out, r)
		return nil
	})
	return out, err
}

// Age is how long ago the report was generated, as of now.
func (r Report) Age(now time.Time) time.Duration {
	return now.Sub(r.Generated)
}
