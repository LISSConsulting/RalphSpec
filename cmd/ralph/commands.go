package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/spec"
)

// loopCmd returns the parent command for autonomous agent loop commands.
func loopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "loop",
		Short: "Autonomous agent loop commands",
	}
	cmd.AddCommand(loopPlanCmd(), loopBuildCmd(), loopRunCmd())
	return cmd
}

func loopPlanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Run an agent in plan mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			max, _ := cmd.Flags().GetInt("max")
			agent, _ := cmd.Flags().GetString("agent")
			noTUI, _ := cmd.Root().PersistentFlags().GetBool("no-tui")
			noColor, _ := cmd.Root().PersistentFlags().GetBool("no-color")
			worktreeFlag, _ := cmd.Flags().GetBool("worktree")
			return executeLoop(loop.ModePlan, max, noTUI, false, "", noColor, worktreeFlag, agent)
		},
	}
	cmd.Flags().Int("max", 0, "override max iterations (0 = use config)")
	cmd.Flags().String("agent", "", "override agent for this run (claude or codex); overrides [agent].type for this invocation only")
	cmd.Flags().BoolP("worktree", "w", false, "run loop in an isolated git worktree via worktrunk")
	return cmd
}

func loopBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Run an agent in build mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			max, _ := cmd.Flags().GetInt("max")
			agent, _ := cmd.Flags().GetString("agent")
			noTUI, _ := cmd.Root().PersistentFlags().GetBool("no-tui")
			noColor, _ := cmd.Root().PersistentFlags().GetBool("no-color")
			roam, _ := cmd.Flags().GetBool("roam")
			focus, _ := cmd.Flags().GetString("focus")
			worktreeFlag, _ := cmd.Flags().GetBool("worktree")
			return executeLoop(loop.ModeBuild, max, noTUI, roam, focus, noColor, worktreeFlag, agent)
		},
	}
	cmd.Flags().Int("max", 0, "override max iterations (0 = use config)")
	cmd.Flags().String("agent", "", "override agent for this run (claude or codex); overrides [agent].type for this invocation only")
	cmd.Flags().Bool("roam", false, "roam freely across the codebase instead of targeting the active spec")
	cmd.Flags().String("focus", "", "constrain roam to a specific topic (e.g. \"UI/UX\")")
	cmd.Flags().BoolP("worktree", "w", false, "run loop in an isolated git worktree via worktrunk")
	return cmd
}

func loopRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Smart mode: plan if needed, then build",
		RunE: func(cmd *cobra.Command, args []string) error {
			max, _ := cmd.Flags().GetInt("max")
			agent, _ := cmd.Flags().GetString("agent")
			noTUI, _ := cmd.Root().PersistentFlags().GetBool("no-tui")
			noColor, _ := cmd.Root().PersistentFlags().GetBool("no-color")
			roam, _ := cmd.Flags().GetBool("roam")
			focus, _ := cmd.Flags().GetString("focus")
			worktreeFlag, _ := cmd.Flags().GetBool("worktree")
			return executeSmartRun(max, noTUI, roam, focus, noColor, worktreeFlag, agent)
		},
	}
	cmd.Flags().Int("max", 0, "override max iterations (0 = use config)")
	cmd.Flags().String("agent", "", "override agent for this run (claude or codex); overrides [agent].type for this invocation only")
	cmd.Flags().Bool("roam", false, "roam freely across the codebase instead of targeting the active spec")
	cmd.Flags().String("focus", "", "constrain roam to a specific topic (e.g. \"UI/UX\")")
	cmd.Flags().BoolP("worktree", "w", false, "run loop in an isolated git worktree via worktrunk")
	return cmd
}

// buildCmd is preserved as a top-level alias for the common build workflow.
func buildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Run an agent in build mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			max, _ := cmd.Flags().GetInt("max")
			agent, _ := cmd.Flags().GetString("agent")
			noTUI, _ := cmd.Root().PersistentFlags().GetBool("no-tui")
			noColor, _ := cmd.Root().PersistentFlags().GetBool("no-color")
			roam, _ := cmd.Flags().GetBool("roam")
			focus, _ := cmd.Flags().GetString("focus")
			worktreeFlag, _ := cmd.Flags().GetBool("worktree")
			return executeLoop(loop.ModeBuild, max, noTUI, roam, focus, noColor, worktreeFlag, agent)
		},
	}
	cmd.Flags().Int("max", 0, "override max iterations (0 = use config)")
	cmd.Flags().String("agent", "", "override agent for this run (claude or codex); overrides [agent].type for this invocation only")
	cmd.Flags().Bool("roam", false, "roam freely across the codebase instead of targeting the active spec")
	cmd.Flags().String("focus", "", "constrain roam to a specific topic (e.g. \"UI/UX\")")
	cmd.Flags().BoolP("worktree", "w", false, "run loop in an isolated git worktree via worktrunk")
	return cmd
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show last run summary from Regent state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return showStatus()
		},
	}
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Scaffold ralph project (config, prompts, specs dir)",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			created, err := config.ScaffoldProject(dir)
			if err != nil {
				return err
			}
			fmt.Print(formatScaffoldResult(created))
			return nil
		},
	}
}

func specCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Manage spec files",
	}
	cmd.AddCommand(specListCmd())
	return cmd
}

func specListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all spec files with status",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			specs, err := spec.List(dir)
			if err != nil {
				return err
			}
			fmt.Print(formatSpecList(specs))
			return nil
		},
	}
}

// formatSpecList renders a list of spec files as a formatted string with
// status symbols. Directory-based features show their Dir path; flat files show
// their .md path. Returns a "no specs" message for empty input.
func formatSpecList(specs []spec.SpecFile) string {
	if len(specs) == 0 {
		return "No specs found in specs/\n"
	}
	var b strings.Builder
	b.WriteString("Specs\n")
	b.WriteString("─────\n")
	for _, s := range specs {
		displayPath := s.Path
		if s.IsDir {
			displayPath = s.Dir
		}
		fmt.Fprintf(&b, "  %s  %-30s  %s\n", s.Status.Symbol(), displayPath, s.Status)
	}
	return b.String()
}

// formatScaffoldResult renders the output of a scaffold operation listing
// created files. Returns an "already exists" message when nothing was created.
func formatScaffoldResult(created []string) string {
	if len(created) == 0 {
		return "All files already exist — nothing to create.\n"
	}
	var b strings.Builder
	for _, path := range created {
		fmt.Fprintf(&b, "Created %s\n", path)
	}
	return b.String()
}
