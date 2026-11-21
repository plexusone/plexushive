// Command plexushive validates and queries a hive directory: the roles the
// long-lived agents play and the decisions that bind them.
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/plexusone/plexushive/hive"
)

func main() {
	if err := newRootCmd(os.Stdout).Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd(out io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:          "plexushive",
		Short:        "Validate and query agent roles and the decision log",
		SilenceUsage: true,
		Version:      version(),
	}
	root.SetOut(out)
	var dir, role string
	root.PersistentFlags().StringVarP(&dir, "dir", "C", ".", "hive directory holding roles/ and decisions/")
	root.PersistentFlags().StringVar(&role, "role", "", "the role this session plays (default $"+hive.RoleEnv+", else the tmux session's)")
	root.AddCommand(validateCmd(&dir), rolesCmd(&dir), schemaCmd(), reportCmd(&dir, &role), briefCmd(&dir),
		decideCmd(&dir), whoCmd(&dir), startCmd(&dir), contextCmd(&dir, &role), whoamiCmd(&role))
	return root
}

func validateCmd(dir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check every role and decision",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := hive.Load(*dir)
			if err != nil {
				return err
			}
			if err := h.Validate(); err != nil {
				return err
			}
			cmd.Printf("%s and %s are valid\n", count(len(h.Roles), "role"), count(len(h.Decisions), "decision"))
			return nil
		},
	}
}

func rolesCmd(dir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "roles github.com/<org>/<repo>",
		Short: "List the roles that work in a repository, steward first",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := hive.Load(*dir)
			if err != nil {
				return err
			}
			matches, err := hive.RolesFor(h.Roles, args[0])
			if err != nil {
				return err
			}
			if len(matches) == 0 {
				cmd.Println("no role lists this repository: it is common ground")
				return nil
			}
			for _, m := range matches {
				scope := "organization"
				if m.Exact {
					scope = "repository"
				}
				cmd.Printf("%-12s %-20s %s\n", m.Affinity, m.Role, scope)
			}
			return nil
		},
	}
}

func schemaCmd() *cobra.Command {
	names := make([]string, 0, len(hive.Schemas))
	for file := range hive.Schemas {
		names = append(names, strings.TrimSuffix(file, ".schema.json"))
	}
	sort.Strings(names)
	return &cobra.Command{
		Use:       "schema " + strings.Join(names, "|"),
		Short:     "Print a record's JSON Schema",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: names,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := hive.Schemas[args[0]+".schema.json"]()
			if err != nil {
				return err
			}
			if _, err := cmd.OutOrStdout().Write(out); err != nil {
				return fmt.Errorf("plexushive: %w", err)
			}
			return nil
		},
	}
}

// count renders n with noun, plural unless n is 1.
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
