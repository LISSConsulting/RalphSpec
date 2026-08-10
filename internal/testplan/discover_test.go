package testplan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverExplicitCommandWins(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"scripts":{"test":"vitest"}}`)
	plan, err := Discover(root, "custom test")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Explicit || len(plan.Steps) != 1 || plan.Steps[0].Command != "custom test" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestDiscoverPackageManagerScript(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "portal/package.json", `{"packageManager":"pnpm@11.17.0","scripts":{"test":"vitest run"}}`)
	plan, err := Discover(root, "")
	if err != nil {
		t.Fatal(err)
	}
	assertStep(t, plan, "portal", "pnpm test", false)
}

func TestDiscoverPythonRequiresDeclaredRunner(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "with-tests/pyproject.toml", "[project]\nname='yes'\ndependencies=['pytest>=8']\n")
	writeFixture(t, root, "without-tests/pyproject.toml", "[project]\nname='no'\ndescription='A pytest integration helper without a test dependency'\n")
	plan, err := Discover(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Command != "python -m pytest" || filepath.Base(plan.Steps[0].Dir) != "with-tests" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestDiscoverMixedLanguageProjects(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "service/go.mod", "module example.com/service\n")
	writeFixture(t, root, "tool/Cargo.toml", "[package]\nname='tool'\nversion='0.1.0'\n")
	plan, err := Discover(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	assertStep(t, plan, "service", "go test ./...", false)
	assertStep(t, plan, "tool", "cargo test", false)
}

func TestAggregateSuppressesChildren(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "justfile", "release-check:\n\tgo test ./...\n")
	writeFixture(t, root, "service/go.mod", "module example.com/service\n")
	writeFixture(t, root, "portal/package.json", `{"scripts":{"test":"vitest"}}`)
	plan, err := Discover(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Command != "just release-check" || !plan.Steps[0].Aggregate {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestWorkspaceSuppressesCargoMembers(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "Cargo.toml", "[workspace]\nmembers=['crates/a']\n")
	writeFixture(t, root, "crates/a/Cargo.toml", "[package]\nname='a'\nversion='0.1.0'\n")
	plan, err := Discover(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Command != "cargo test --workspace" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestRootJustfileWinsOverWorkspaceAggregate(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "justfile", "release-check:\n\tgo test ./...\n")
	writeFixture(t, root, "Cargo.toml", "[workspace]\nmembers=['crates/a']\n")
	plan, err := Discover(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Command != "just release-check" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestNestedJustfileDoesNotSuppressProjectMetadata(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "service/justfile", "release-check:\n\tgo test ./...\n")
	writeFixture(t, root, "service/go.mod", "module example.com/service\n")
	plan, err := Discover(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Command != "go test ./..." || plan.Steps[0].Aggregate {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestIgnoredAndMalformedMetadata(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "node_modules/pkg/package.json", `{"scripts":{"test":"bad"}}`)
	writeFixture(t, root, "package.json", `{bad`)
	plan, err := Discover(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 0 || len(plan.Diagnostics) < 2 {
		t.Fatalf("plan = %#v", plan)
	}
}

func writeFixture(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertStep(t *testing.T, plan Plan, dir, command string, aggregate bool) {
	t.Helper()
	for _, step := range plan.Steps {
		if filepath.Base(step.Dir) == dir && step.Command == command && step.Aggregate == aggregate {
			return
		}
	}
	t.Fatalf("missing %s/%s in %#v", dir, command, plan.Steps)
}
