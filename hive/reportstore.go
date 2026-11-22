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

// WriteReport normalizes and validates r and writes it to dir as
// <identity>.json, replacing the work identity's previous report. The write is
// atomic: a reader sees the old report or the new one, never part of either.
func WriteReport(dir string, r Report) error {
	r.Normalize()
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
	if err := fs.WriteFileAtomic(filepath.Join(dir, r.Identity+".json"), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("hive: %w", err)
	}
	return nil
}

// LoadReports reads every <identity>.json in dir and fails on any report it
// cannot read. A directory that does not exist yet holds no reports.
func LoadReports(dir string) ([]Report, error) {
	reports, unreadable, err := ReadReports(dir)
	if err != nil {
		return nil, err
	}
	if len(unreadable) > 0 {
		return nil, fmt.Errorf("hive: %s", unreadable[0])
	}
	return reports, nil
}

// UnreadableReport is a report file that could not be decoded, such as one
// written by a newer version with fields this version does not know.
type UnreadableReport struct {
	Identity string `json:"identity"`
	Error    string `json:"error"`
}

func (u UnreadableReport) String() string {
	return u.Identity + ".json: " + u.Error
}

// ReadReports reads every <identity>.json in dir. A report that cannot be
// decoded is returned in unreadable rather than failing the rest, so one work
// identity writing a newer format does not stop every other work identity from
// reading the briefing. err is set only when the directory or a file cannot be
// read at all.
func ReadReports(dir string) (reports []Report, unreadable []UnreadableReport, err error) {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	err = eachJSON(dir, func(stem string, data []byte) error {
		var r Report
		if err := decodeStrict(data, &r); err != nil {
			unreadable = append(unreadable, UnreadableReport{Identity: stem, Error: err.Error()})
			return nil
		}
		if r.Identity != stem {
			unreadable = append(unreadable, UnreadableReport{Identity: stem, Error: fmt.Sprintf("work identity %q does not match the file name", r.Identity)})
			return nil
		}
		reports = append(reports, r)
		return nil
	})
	return reports, unreadable, err
}

// Age is how long ago the report was generated, as of now.
func (r Report) Age(now time.Time) time.Duration {
	return now.Sub(r.Generated)
}
