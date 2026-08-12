package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/LISSConsulting/RalphSpec/internal/config"
)

const ludicrousHelp = `Ludicrous mode keeps a spec-bound build running until Ralph can verify goal completion instead of trusting a single agent claim.

Without an explicit positive --max, ludicrous mode ignores build.max_iterations. A positive --max remains a hard limit; reaching it without verified completion returns an error.

Verified completion requires:
  1. Two consecutive structured agent-success outcomes
  2. No commit during the confirming iteration
  3. A clean worktree
  4. All declared spec tasks complete, when an active spec exists
  5. The selected test plan passing

Configure verification with regent.test_command, or set regent.auto_discover_tests=true to detect tests from project metadata. Run "ralph tests detect" to preview the immutable plan.

Quota admission, permissions, safety controls, and operator stops remain active. Ludicrous mode is for spec-bound builds and cannot be combined with --roam.

Persistent configuration:
  [build]
  ludicrous = true`

const ludicrousExamples = `  ralph build --ludicrous
  ralph build --ludicrous --no-tui
  ralph build --ludicrous --max 25
  ralph build --ludicrous --agent codex
  ralph loop run --ludicrous`

type commandHelpDocument struct {
	SchemaVersion string                `json:"schema_version"`
	Name          string                `json:"name"`
	Path          string                `json:"path"`
	Usage         string                `json:"usage"`
	Short         string                `json:"short,omitempty"`
	Long          string                `json:"long,omitempty"`
	Example       string                `json:"example,omitempty"`
	Flags         []flagHelpDocument    `json:"flags,omitempty"`
	Commands      []commandHelpDocument `json:"commands,omitempty"`
}

type flagHelpDocument struct {
	Name       string `json:"name"`
	Shorthand  string `json:"shorthand,omitempty"`
	Type       string `json:"type"`
	Default    any    `json:"default,omitempty"`
	Usage      string `json:"usage"`
	Scope      string `json:"scope"`
	NoOptValue string `json:"no_opt_value,omitempty"`
}

func configureHelp(root *cobra.Command) {
	var jsonOutput bool
	help := &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Long:  "Show human-readable command help, or emit a stable command tree for tools and coding agents with --json.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := root
			if len(args) > 0 {
				found, remaining, err := root.Find(args)
				if err != nil {
					return err
				}
				if len(remaining) > 0 {
					return fmt.Errorf("unknown help topic %q", strings.Join(args, " "))
				}
				target = found
			}
			if !jsonOutput {
				return target.Help()
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(buildCommandHelp(target))
		},
	}
	help.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable command help as JSON")
	root.SetHelpCommand(help)
}

func buildCommandHelp(cmd *cobra.Command) commandHelpDocument {
	doc := commandHelpDocument{
		SchemaVersion: "1",
		Name:          cmd.Name(),
		Path:          cmd.CommandPath(),
		Usage:         cmd.UseLine(),
		Short:         cmd.Short,
		Long:          cmd.Long,
		Example:       cmd.Example,
	}
	doc.Flags = appendFlagHelp(doc.Flags, cmd.NonInheritedFlags(), "local")
	doc.Flags = appendFlagHelp(doc.Flags, cmd.InheritedFlags(), "inherited")
	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		doc.Commands = append(doc.Commands, buildCommandHelp(child))
	}
	return doc
}

func appendFlagHelp(dst []flagHelpDocument, flags *pflag.FlagSet, scope string) []flagHelpDocument {
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		item := flagHelpDocument{
			Name:       flag.Name,
			Shorthand:  flag.Shorthand,
			Type:       flag.Value.Type(),
			Default:    typedFlagDefault(flag),
			Usage:      flag.Usage,
			Scope:      scope,
			NoOptValue: flag.NoOptDefVal,
		}
		dst = append(dst, item)
	})
	return dst
}

func typedFlagDefault(flag *pflag.Flag) any {
	switch flag.Value.Type() {
	case "bool":
		value, err := strconv.ParseBool(flag.DefValue)
		if err == nil {
			return value
		}
	case "int", "int32", "int64":
		value, err := strconv.ParseInt(flag.DefValue, 10, 64)
		if err == nil {
			return value
		}
	case "float32", "float64":
		value, err := strconv.ParseFloat(flag.DefValue, 64)
		if err == nil {
			return value
		}
	}
	return flag.DefValue
}

func ludicrousCmd() *cobra.Command {
	return &cobra.Command{
		Use:                   "ludicrous",
		Short:                 "Explain evidence-gated goal-persistent build mode",
		Long:                  ludicrousHelp,
		Example:               ludicrousExamples,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
}

func configCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Inspect Ralph configuration contracts"}
	cmd.AddCommand(configSchemaCmd())
	return cmd
}

func configSchemaCmd() *cobra.Command {
	var compact bool
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Emit the ralph.toml contract as JSON Schema",
		Long:  "Emit a generated JSON Schema containing every ralph.toml key, type, default, enum, and critical behavior description.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			encoder := json.NewEncoder(cmd.OutOrStdout())
			if !compact {
				encoder.SetIndent("", "  ")
			}
			return encoder.Encode(buildConfigSchema())
		},
	}
	cmd.Flags().BoolVar(&compact, "json", false, "emit compact JSON for machine consumption")
	return cmd
}

func buildConfigSchema() map[string]any {
	defaults := reflect.ValueOf(config.Defaults())
	schema := configSchemaForValue(defaults, "")
	schema["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	schema["$id"] = "https://github.com/LISSConsulting/RalphSpec/config.schema.json"
	schema["title"] = "RalphSpec ralph.toml configuration"
	schema["description"] = "Machine-readable configuration contract. All properties are optional and use the documented defaults. Unknown properties are rejected."
	return schema
}

func configSchemaForValue(value reflect.Value, path string) map[string]any {
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Struct:
		properties := make(map[string]any, value.NumField())
		typeInfo := value.Type()
		for i := range value.NumField() {
			name := strings.Split(typeInfo.Field(i).Tag.Get("toml"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			childPath := name
			if path != "" {
				childPath = path + "." + name
			}
			properties[name] = configSchemaForValue(value.Field(i), childPath)
		}
		result := map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           properties,
		}
		if description := configDescriptions[path]; description != "" {
			result["description"] = description
		}
		return result
	case reflect.Bool:
		return configScalarSchema("boolean", value.Bool(), path)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return configScalarSchema("integer", value.Int(), path)
	case reflect.Float32, reflect.Float64:
		return configScalarSchema("number", value.Float(), path)
	case reflect.String:
		return configScalarSchema("string", value.String(), path)
	case reflect.Slice:
		if value.Type().Elem().Kind() != reflect.String {
			panic(fmt.Sprintf("unsupported config schema slice type %s at %s", value.Type(), path))
		}
		defaultValue := make([]string, value.Len())
		for i := range value.Len() {
			defaultValue[i] = value.Index(i).String()
		}
		result := map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string", "minLength": 1},
			"default":     defaultValue,
			"uniqueItems": true,
		}
		if description := configDescriptions[path]; description != "" {
			result["description"] = description
		}
		return result
	default:
		panic(fmt.Sprintf("unsupported config schema type %s at %s", value.Kind(), path))
	}
}

func configScalarSchema(kind string, defaultValue any, path string) map[string]any {
	result := map[string]any{"type": kind, "default": defaultValue}
	if description := configDescriptions[path]; description != "" {
		result["description"] = description
	}
	if values := configEnums[path]; len(values) > 0 {
		result["enum"] = values
	}
	if bounds := configBounds[path]; bounds != nil {
		for key, value := range bounds {
			result[key] = value
		}
	}
	return result
}

var configEnums = map[string][]string{
	"agent.type":             {"", config.AgentClaude, config.AgentCodex},
	"quota.exhausted_policy": {config.QuotaFailClosed, config.QuotaWait},
	"quota.unknown_policy":   {config.QuotaAllow, config.QuotaFailClosed},
}

var configBounds = map[string]map[string]any{
	"build.prompt_file":                     {"minLength": 1},
	"build.max_iterations":                  {"minimum": 0},
	"roam.prompt_file":                      {"minLength": 1},
	"roam.max_iterations":                   {"minimum": 0},
	"claude.max_turns":                      {"minimum": 0},
	"claude.quota_snapshot_max_age_seconds": {"minimum": 0},
	"quota.reserve_percent":                 {"minimum": 0, "maximum": 100},
	"quota.max_wait_seconds":                {"minimum": 0},
	"quota.max_parallel":                    {"minimum": 1},
	"regent.max_retries":                    {"minimum": 0},
	"regent.retry_backoff_seconds":          {"minimum": 0},
	"regent.hang_timeout_seconds":           {"minimum": 0},
	"tui.accent_color":                      {"pattern": `^$|^#[0-9A-Fa-f]{6}$`},
	"tui.log_retention":                     {"minimum": 0},
	"notifications.url":                     {"pattern": `^$|^https?://`},
	"worktree.max_parallel":                 {"minimum": 1},
}

var configDescriptions = map[string]string{
	"project":                               "Project identity.",
	"project.name":                          "Display name; an empty value is detected from project metadata or the directory name.",
	"agent":                                 "Default coding-agent selection.",
	"agent.type":                            "Default supported harness; an empty value resolves to Claude.",
	"agent.stdin_steer":                     "Experimental mid-turn forwarding of TUI steering messages to agent stdin.",
	"harness":                               "Executable names or shims for supported harnesses.",
	"claude.quota_snapshot_file":            "Operator-maintained Claude status-line JSON snapshot used for quota admission.",
	"claude.quota_snapshot_max_age_seconds": "Maximum accepted age of the Claude quota snapshot.",
	"claude.provider":                       "Display label for the inherited primary Claude provider environment.",
	"claude.provider_config_file":           "AIProvider-compatible JSON profile file; empty defaults to ~/.claude/providers.json.",
	"claude.fallback_providers":             "Ordered provider profiles attempted within the same iteration only after normalized quota exhaustion.",
	"claude.max_turns":                      "Maximum Claude agent turns per iteration; 0 means unlimited.",
	"quota":                                 "Admission policy for starting agent work; it never throttles a request already in flight.",
	"quota.enabled":                         "Enable quota checks before iterations and parallel worker launches.",
	"quota.reserve_percent":                 "Percentage of each quota window reserved instead of admitted for new work.",
	"quota.exhausted_policy":                "Block immediately or wait for the provider reset when known.",
	"quota.unknown_policy":                  "Admission behavior when provider quota cannot be observed.",
	"quota.max_wait_seconds":                "Maximum bounded wait for quota reset.",
	"quota.max_parallel":                    "Shared concurrent worker cap per agent type.",
	"build":                                 "Spec-bound build-loop behavior.",
	"build.prompt_file":                     "Prompt template used for build iterations.",
	"build.max_iterations":                  "Configured iteration cap; 0 means unlimited. Ludicrous mode ignores this unless a positive --max is supplied.",
	"build.ludicrous":                       "Persistently enable evidence-gated goal-persistent mode for spec-bound builds.",
	"roam":                                  "Codebase-wide roaming behavior; incompatible with ludicrous mode.",
	"roam.prompt_file":                      "Prompt template used for roaming iterations.",
	"roam.max_iterations":                   "Roaming iteration cap; 0 means unlimited.",
	"regent":                                "Supervisor, retry, and verification behavior.",
	"regent.test_command":                   "Authoritative verification command. When set, it takes precedence over metadata discovery.",
	"regent.auto_discover_tests":            "Detect an immutable high-confidence test plan from project metadata when test_command is empty.",
	"regent.max_retries":                    "Maximum retries for ordinary failures; quota and authentication blocks do not consume retries.",
	"regent.retry_backoff_seconds":          "Delay between retries for ordinary failures.",
	"regent.hang_timeout_seconds":           "No-output timeout; 0 disables hang detection.",
	"git.auto_push":                         "Push after successful agent commits.",
	"worktree.max_parallel":                 "Maximum number of concurrent worktree agents before quota limits are applied.",
	"worktree":                              "Parallel isolated-worktree orchestration.",
	"notifications.url":                     "Optional HTTP or HTTPS webhook notification endpoint.",
	"tui.accent_color":                      "Six-digit hexadecimal accent color, or empty for the default.",
}
