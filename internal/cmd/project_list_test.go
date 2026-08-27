package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
)

func TestProjectList_Table(t *testing.T) {
	setupProjectTest(t, []api.Project{
		{ID: "project-old", Name: "Old Project"},
		{ID: "project-new", Name: "New Project"},
	}, "project-old", "")

	output, err := executeAuthCommand(t.Context(), "project", "list")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	for _, want := range []string{"CURRENT", "PROJECT ID", "NAME", "project-old", "Old Project", "project-new", "New Project"} {
		if !strings.Contains(output, want) {
			t.Errorf("Expected output to contain %q, got: %q", want, output)
		}
	}

	// Only the active project is marked.
	for line := range strings.SplitSeq(output, "\n") {
		if !strings.Contains(line, "project-old") && !strings.Contains(line, "project-new") {
			continue
		}
		marked := strings.Contains(line, "*")
		if strings.Contains(line, "project-old") && !marked {
			t.Errorf("Expected the active project to be marked, got: %q", line)
		}
		if strings.Contains(line, "project-new") && marked {
			t.Errorf("Expected non-active project to be unmarked, got: %q", line)
		}
	}
}

func TestProjectList_JSON(t *testing.T) {
	setupProjectTest(t, []api.Project{
		{ID: "project-old", Name: "Old Project"},
		{ID: "project-new", Name: "New Project"},
	}, "project-new", "")

	output, err := executeAuthCommand(t.Context(), "project", "list", "--output", "json")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	var projects []OutputProject
	if err := json.Unmarshal([]byte(output), &projects); err != nil {
		t.Fatalf("Failed to parse JSON output %q: %v", output, err)
	}

	if len(projects) != 2 {
		t.Fatalf("Expected 2 projects, got %d: %+v", len(projects), projects)
	}
	if projects[0].ID != "project-old" || projects[0].Current {
		t.Errorf("Unexpected first project: %+v", projects[0])
	}
	if projects[1].ID != "project-new" || !projects[1].Current {
		t.Errorf("Unexpected second project: %+v", projects[1])
	}
}

func TestProjectList_Alias(t *testing.T) {
	setupProjectTest(t, []api.Project{
		{ID: "project-old", Name: "Old Project"},
	}, "project-old", "")

	output, err := executeAuthCommand(t.Context(), "project", "ls")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if !strings.Contains(output, "project-old") {
		t.Errorf("Expected project in output, got: %q", output)
	}
}

func TestProjectList_EnvFormatRejected(t *testing.T) {
	setupProjectTest(t, []api.Project{
		{ID: "project-old", Name: "Old Project"},
	}, "project-old", "")
	// --output rejects env at parse time, but TIGER_OUTPUT isn't validated on load.
	t.Setenv("TIGER_OUTPUT", "env")

	_, err := executeAuthCommand(t.Context(), "project", "list")
	if err == nil {
		t.Fatal("Expected error for env output format")
	}
	if !strings.Contains(err.Error(), "environment variable output is not supported for multiple projects") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestProjectList_NotLoggedIn(t *testing.T) {
	setupAuthTest(t)

	_, err := executeAuthCommand(t.Context(), "project", "list")
	if err == nil {
		t.Fatal("Expected error when not logged in")
	}
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Unexpected error: %v", err)
	}
}
