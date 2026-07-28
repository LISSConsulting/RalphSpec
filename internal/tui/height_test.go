package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/spec"
)

func TestViewHeight_MatchesTerminal(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	close(ch)
	// Use long spec names that exceed typical sidebar width to catch wrapping bugs.
	specs := []spec.SpecFile{
		{Name: "012-encryption-service-isolation", IsDir: true, Dir: "specs/012-encryption-service-isolation", Path: "specs/012-encryption-service-isolation/spec.md", Status: spec.StatusTasked},
		{Name: "013-clients-domain", IsDir: true, Dir: "specs/013-clients-domain", Path: "specs/013-clients-domain/spec.md", Status: spec.StatusTasked},
		{Name: "014-rbac-unification", IsDir: true, Dir: "specs/014-rbac-unification", Path: "specs/014-rbac-unification/spec.md", Status: spec.StatusTasked},
		{Name: "015-domain-settings-registry", IsDir: true, Dir: "specs/015-domain-settings-registry", Path: "specs/015-domain-settings-registry/spec.md", Status: spec.StatusTasked},
		{Name: "016-account-review-report", IsDir: true, Dir: "specs/016-account-review-report", Path: "specs/016-account-review-report/spec.md", Status: spec.StatusTasked},
		{Name: "017-orchestramsp-rebrand", IsDir: true, Dir: "specs/017-orchestramsp-rebrand", Path: "specs/017-orchestramsp-rebrand/spec.md", Status: spec.StatusTasked},
		{Name: "018-frontend-redesign", IsDir: true, Dir: "specs/018-frontend-redesign", Path: "specs/018-frontend-redesign/spec.md", Status: spec.StatusTasked},
		{Name: "019-integrations", IsDir: true, Dir: "specs/019-integrations", Path: "specs/019-integrations/spec.md", Status: spec.StatusSpecified},
		{Name: "020-auth-domain-permissions", IsDir: true, Dir: "specs/020-auth-domain-permissions", Path: "specs/020-auth-domain-permissions/spec.md", Status: spec.StatusSpecified},
		{Name: "021-page-restyle", IsDir: true, Dir: "specs/021-page-restyle", Path: "specs/021-page-restyle/spec.md", Status: spec.StatusSpecified},
	}

	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 30}, {160, 40}, {200, 50}} {
		m := New(ch, nil, "#7D56F4", "TestProject", "/tmp/project", specs, nil, nil, nil)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
		m = updated.(Model)
		lines := strings.Split(m.View(), "\n")
		if len(lines) != sz.h {
			t.Errorf("Terminal %dx%d: rendered %d lines, expected %d (overflow %d)",
				sz.w, sz.h, len(lines), sz.h, len(lines)-sz.h)
		}
	}
}
