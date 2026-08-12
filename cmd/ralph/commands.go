package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/spec"
	"github.com/LISSConsulting/RalphSpec/internal/testplan"
)

// loopCmd returns the parent command for autonomous agent loop commands.
func loopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "loop",
		Short: "Autonomous agent loop commands",
	}
	cmd.AddCommand(loopBuildCmd(), loopRunCmd())
	return cmd
}

func loopBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "build",
		Short:   "Run an agent in build mode",
		Long:    "Run an agent in build mode.\n\n" + ludicrousHelp,
		Example: ludicrousExamples,
		RunE: func(cmd *cobra.Command, args []string) error {
			max, _ := cmd.Flags().GetInt("max")
			agent, _ := cmd.Flags().GetString("agent")
			noTUI, _ := cmd.Root().PersistentFlags().GetBool("no-tui")
			noColor, _ := cmd.Root().PersistentFlags().GetBool("no-color")
			roam, _ := cmd.Flags().GetBool("roam")
			focus, _ := cmd.Flags().GetString("focus")
			ludicrous, _ := cmd.Flags().GetBool("ludicrous")
			worktreeFlag, _ := cmd.Flags().GetBool("worktree")
			return executeLoopWithOptions(loop.ModeBuild, max, noTUI, roam, focus, noColor, worktreeFlag, agent, ludicrous)
		},
	}
	cmd.Flags().Int("max", 0, "override max iterations (0 = use config)")
	cmd.Flags().String("agent", "", "override agent (claude or codex); overrides [agent].type for this run")
	cmd.Flags().Bool("roam", false, "roam freely across the codebase instead of targeting the active spec")
	cmd.Flags().String("focus", "", "constrain roam to a specific topic (e.g. \"UI/UX\")")
	cmd.Flags().BoolP("worktree", "w", false, "run loop in an isolated git worktree via worktrunk")
	cmd.Flags().Bool("ludicrous", false, "continue until structured completion evidence and required tests pass")
	return cmd
}

func loopRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "run",
		Short:   "Run the agent loop; use --roam for codebase-wide roaming",
		Long:    "Run the spec-bound agent loop; use --roam for codebase-wide roaming.\n\n" + ludicrousHelp,
		Example: ludicrousExamples,
		RunE: func(cmd *cobra.Command, args []string) error {
			max, _ := cmd.Flags().GetInt("max")
			agent, _ := cmd.Flags().GetString("agent")
			noTUI, _ := cmd.Root().PersistentFlags().GetBool("no-tui")
			noColor, _ := cmd.Root().PersistentFlags().GetBool("no-color")
			roam, _ := cmd.Flags().GetBool("roam")
			focus, _ := cmd.Flags().GetString("focus")
			worktreeFlag, _ := cmd.Flags().GetBool("worktree")
			ludicrous, _ := cmd.Flags().GetBool("ludicrous")
			return executeRunWithOptions(max, noTUI, roam, focus, noColor, worktreeFlag, agent, ludicrous)
		},
	}
	cmd.Flags().Int("max", 0, "override max iterations (0 = use config)")
	cmd.Flags().String("agent", "", "override agent (claude or codex); overrides [agent].type for this run")
	cmd.Flags().Bool("roam", false, "roam freely across the codebase instead of targeting the active spec")
	cmd.Flags().String("focus", "", "constrain roam to a specific topic (e.g. \"UI/UX\")")
	cmd.Flags().BoolP("worktree", "w", false, "run loop in an isolated git worktree via worktrunk")
	cmd.Flags().Bool("ludicrous", false, "continue until verified completion evidence passes; unbounded unless --max is positive")
	return cmd
}

// buildCmd is preserved as a top-level alias for the common build workflow.
func buildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "build",
		Short:   "Run an agent in build mode",
		Long:    "Run an agent in build mode.\n\n" + ludicrousHelp,
		Example: ludicrousExamples,
		RunE: func(cmd *cobra.Command, args []string) error {
			max, _ := cmd.Flags().GetInt("max")
			agent, _ := cmd.Flags().GetString("agent")
			noTUI, _ := cmd.Root().PersistentFlags().GetBool("no-tui")
			noColor, _ := cmd.Root().PersistentFlags().GetBool("no-color")
			roam, _ := cmd.Flags().GetBool("roam")
			focus, _ := cmd.Flags().GetString("focus")
			worktreeFlag, _ := cmd.Flags().GetBool("worktree")
			ludicrous, _ := cmd.Flags().GetBool("ludicrous")
			return executeLoopWithOptions(loop.ModeBuild, max, noTUI, roam, focus, noColor, worktreeFlag, agent, ludicrous)
		},
	}
	cmd.Flags().Int("max", 0, "override max iterations (0 = use config)")
	cmd.Flags().String("agent", "", "override agent (claude or codex); overrides [agent].type for this run")
	cmd.Flags().Bool("roam", false, "roam freely across the codebase instead of targeting the active spec")
	cmd.Flags().String("focus", "", "constrain roam to a specific topic (e.g. \"UI/UX\")")
	cmd.Flags().BoolP("worktree", "w", false, "run loop in an isolated git worktree via worktrunk")
	cmd.Flags().Bool("ludicrous", false, "continue until verified completion evidence passes; unbounded unless --max is positive")
	return cmd
}

func testsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tests", Short: "Discover and run project test plans"}
	cmd.AddCommand(testDetectCmd(), testRunCmd())
	return cmd
}

func testDetectCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Preview the deterministic project test plan without executing it",
		RunE: func(cmd *cobra.Command, _ []string) error {
			plan, root, err := discoverConfiguredTestPlan()
			if err != nil {
				return err
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
			}
			if err := printTestPlan(cmd, root, plan); err != nil {
				return err
			}
			if len(plan.Steps) == 0 {
				return fmt.Errorf("test plan discovery was inconclusive")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "write the plan as JSON")
	return cmd
}

func testRunCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute the deterministic high-confidence project test plan",
		RunE: func(cmd *cobra.Command, _ []string) error {
			plan, root, err := discoverConfiguredTestPlan()
			if err != nil {
				return err
			}
			if len(plan.Steps) == 0 {
				if !jsonOutput {
					if err := printTestPlan(cmd, root, plan); err != nil {
						return err
					}
				}
				return fmt.Errorf("test plan discovery was inconclusive")
			}
			result := testplan.Run(context.Background(), plan)
			if jsonOutput {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(result); err != nil {
					return err
				}
			} else {
				for _, step := range result.Steps {
					status := "PASS"
					if !step.Passed {
						status = "FAIL"
					}
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s\n", status, displayDir(root, step.Step.Dir), step.Step.Command); err != nil {
						return err
					}
					if step.Output != "" {
						if _, err := fmt.Fprint(cmd.OutOrStdout(), step.Output); err != nil {
							return err
						}
					}
				}
			}
			if !result.Passed {
				return fmt.Errorf("test plan failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "write execution results as JSON")
	return cmd
}

func discoverConfiguredTestPlan() (testplan.Plan, string, error) {
	root, err := os.Getwd()
	if err != nil {
		return testplan.Plan{}, "", err
	}
	explicit := ""
	if cfg, loadErr := config.Load(""); loadErr == nil {
		explicit = cfg.Regent.TestCommand
	}
	plan, err := testplan.Discover(root, explicit)
	return plan, root, err
}

func printTestPlan(cmd *cobra.Command, root string, plan testplan.Plan) error {
	for _, step := range plan.Steps {
		label := strings.ToUpper(string(step.Confidence))
		if step.Aggregate {
			label += " aggregate"
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-14s %s  %s\n", label, displayDir(root, step.Dir), step.Command); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "               source: %s\n", step.Source); err != nil {
			return err
		}
	}
	for _, diagnostic := range plan.Diagnostics {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "diagnostic: %s\n", diagnostic); err != nil {
			return err
		}
	}
	return nil
}

func displayDir(root, dir string) string {
	relative, err := filepath.Rel(root, dir)
	if err == nil {
		if relative == "." {
			return "."
		}
		return relative
	}
	return dir
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
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold ralph project (config, prompts, specs dir)",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			created, err := config.ScaffoldProjectWithOptions(dir, config.ScaffoldOptions{Force: force})
			if err != nil {
				return err
			}
			fmt.Print(formatScaffoldResult(created))
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite Ralph scaffold files and remove legacy PLAN.md")
	return cmd
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
