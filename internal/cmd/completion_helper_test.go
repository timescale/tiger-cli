package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

// TestCompletion covers the shell completion functions through the
// `__complete` command cobra installs, which is what a real shell invokes.
//
// Cobra writes the candidates to stdout, one "value\tdescription" line each,
// followed by a ":<directive>" line — 4 is ShellCompDirectiveNoFileComp, which
// nearly every completion here returns so shells don't fall back to filenames;
// 8 and 16 filter to files and directories. The trailing human-readable
// directive line goes to stderr.
//
// A completion that can't produce candidates (not logged in, API error) must
// return none rather than an error: a broken completion should be invisible at
// the prompt, not print a stack of errors over the user's command line.
func TestCompletion(t *testing.T) {
	const noFileComp = ":4\n"
	const directive = "Completion ended with directive: ShellCompDirectiveNoFileComp\n"

	listServices := func(m *mocks.MockClientWithResponsesInterface) {
		services := []api.Service{
			sampleService(),
			sampleService(func(s *api.Service) {
				s.ServiceID = "svc-67890"
				s.Name = "other-service"
			}),
		}
		m.EXPECT().GetServicesWithResponse(validCtx, testProjectID).
			Return(&api.GetServicesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &services,
			}, nil)
	}

	runCmdTests(t, []cmdTest{
		{
			name:       "service ID lists services with their names",
			args:       []string{"__complete", "service", "get", ""},
			setup:      listServices,
			wantStdout: "svc-12345\ttest-service\nsvc-67890\tother-service\n" + noFileComp,
			wantStderr: directive,
		},
		{
			name:       "service ID filters by what is typed",
			args:       []string{"__complete", "service", "get", "svc-67"},
			setup:      listServices,
			wantStdout: "svc-67890\tother-service\n" + noFileComp,
			wantStderr: directive,
		},
		{
			// The service ID is the only positional argument, so there is
			// nothing to complete once one is present.
			name:       "service ID completes only the first argument",
			args:       []string{"__complete", "service", "get", "svc-12345", ""},
			wantStdout: noFileComp,
			wantStderr: directive,
		},
		{
			name:       "service ID offers nothing when not logged in",
			args:       []string{"__complete", "service", "get", ""},
			opts:       []runOption{withNotLoggedIn()},
			wantStdout: noFileComp,
			wantStderr: directive,
		},
		{
			name: "service ID offers nothing when the API fails",
			args: []string{"__complete", "service", "get", ""},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServicesWithResponse(validCtx, testProjectID).
					Return(nil, errors.New("connection refused"))
			},
			wantStdout: noFileComp,
			wantStderr: directive,
		},
		{
			name:       "--service-id lists services with their names",
			args:       []string{"__complete", "service", "get", "--service-id", ""},
			setup:      listServices,
			wantStdout: "svc-12345\ttest-service\nsvc-67890\tother-service\n" + noFileComp,
			wantStderr: directive,
		},
		{
			// The flag isn't positional, so unlike the argument it still
			// completes once a service ID has been typed.
			name:       "--service-id completes after a positional argument",
			args:       []string{"__complete", "service", "get", "svc-12345", "--service-id", ""},
			setup:      listServices,
			wantStdout: "svc-12345\ttest-service\nsvc-67890\tother-service\n" + noFileComp,
			wantStderr: directive,
		},
		{
			name: "project ID lists projects with their names",
			args: []string{"__complete", "project", "use", ""},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				projects := []api.Project{
					{ID: "project-123", Name: "First Project"},
					{ID: "project-456", Name: "Second Project"},
				}
				m.EXPECT().GetProjectsWithResponse(validCtx).
					Return(&api.GetProjectsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &projects,
					}, nil)
			},
			wantStdout: "project-123\tFirst Project\nproject-456\tSecond Project\n" + noFileComp,
			wantStderr: directive,
		},
		{
			// Config keys come straight from the registry, so this list is the
			// same one `tiger config show` renders (see TestConfigShowCoversEveryKey).
			name:       "config set completes keys",
			args:       []string{"__complete", "config", "set", ""},
			wantStdout: "analytics\napi_url\ncolor\nconsole_url\ndocs_mcp\ndocs_mcp_url\ngateway_url\nmcp_max_rows\noutput\npassword_storage\nread_only\nreleases_url\nservice_id\nversion_check\n" + noFileComp,
			wantStderr: directive,
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:       "config set completes a key's values",
			args:       []string{"__complete", "config", "set", "output", ""},
			wantStdout: "json\nyaml\ntable\n" + noFileComp,
			wantStderr: directive,
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			// A free-form key has no fixed set of values to offer.
			name:       "config set offers no values for a free-form key",
			args:       []string{"__complete", "config", "set", "service_id", ""},
			wantStdout: noFileComp,
			wantStderr: directive,
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:       "config unset completes keys",
			args:       []string{"__complete", "config", "unset", "docs"},
			wantStdout: "docs_mcp\ndocs_mcp_url\n" + noFileComp,
			wantStderr: directive,
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:       "--environment flag values",
			args:       []string{"__complete", "service", "create", "--environment", ""},
			wantStdout: "DEV\nPROD\n" + noFileComp,
			wantStderr: directive,
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:       "--addons flag values",
			args:       []string{"__complete", "service", "create", "--addons", ""},
			wantStdout: "time-series\nai\nnone\n" + noFileComp,
			wantStderr: directive,
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:       "--password-storage flag values",
			args:       []string{"__complete", "service", "get", "--password-storage", ""},
			wantStdout: "keyring\npgpass\nnone\n" + noFileComp,
			wantStderr: directive,
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			// --output's candidates are per-command: `service get` adds "env"
			// to the universal three, `version` adds "bare".
			name:       "--output flag values include the command's extra formats",
			args:       []string{"__complete", "service", "get", "--output", ""},
			wantStdout: "json\nyaml\ntable\nenv\n" + noFileComp,
			wantStderr: directive,
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:       "--output flag values for version",
			args:       []string{"__complete", "version", "--output", ""},
			wantStdout: "json\nyaml\ntable\nbare\n" + noFileComp,
			wantStderr: directive,
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:       "mcp get completes capability names",
			args:       []string{"__complete", "mcp", "get", "service_l"},
			opts:       noDocsProxy(nil),
			wantStdout: "service_list\nservice_logs\n" + noFileComp,
			wantStderr: directive,
		},
		{
			// The root command's default directive, which is what keeps every
			// flag without a registered completion from offering filenames.
			name:       "a flag with no completion offers nothing",
			args:       []string{"__complete", "service", "create", "--name", ""},
			wantStdout: noFileComp,
			wantStderr: directive,
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:       "a command with no argument completion offers nothing",
			args:       []string{"__complete", "service", "list", ""},
			wantStdout: noFileComp,
			wantStderr: directive,
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:       "--config-dir completes directories",
			args:       []string{"__complete", "service", "get", "--config-dir", ""},
			wantStdout: ":16\n",
			wantStderr: "Completion ended with directive: ShellCompDirectiveFilterDirs\n",
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			// A client's config file can be named anything, so this asks for
			// the shell's unfiltered file completion.
			name:       "--config-path completes any file",
			args:       []string{"__complete", "mcp", "install", "--config-path", ""},
			wantStdout: ":0\n",
			wantStderr: "Completion ended with directive: ShellCompDirectiveDefault\n",
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			// Same for a SQL file.
			name:       "--file completes any file",
			args:       []string{"__complete", "db", "query", "--file", ""},
			wantStdout: ":0\n",
			wantStderr: "Completion ended with directive: ShellCompDirectiveDefault\n",
			checks:     []checkFunc{checkNotLoaded},
		},
	})

	// Completing a command name touches nothing but the command tree, which is
	// the whole reason wrapCommands leaves __complete unwrapped.
	t.Run("completing subcommand names loads nothing", func(t *testing.T) {
		result := runCommand(t, []string{"__complete", "serv"}, nil)
		assertOutput(t, result.stdout, "service\tManage database services\n"+noFileComp)
		checkNotLoaded(t, result)
	})

	// Same for help, which cobra also adds after wrapCommands has run.
	t.Run("help loads nothing", func(t *testing.T) {
		result := runCommand(t, []string{"--help"}, nil)
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		checkNotLoaded(t, result)
	})
}

// TestOutputFlagCompletion asserts that every --output flag in the tree has a
// completion registered. Defining the flag and registering its completion are
// two separate lines, so it's easy to add the first and forget the second —
// this is the check that would catch that.
func TestOutputFlagCompletion(t *testing.T) {
	root, _, err := buildRootCmd(t.Context())
	if err != nil {
		t.Fatalf("buildRootCmd failed: %v", err)
	}

	var missing []string
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Flags().Lookup("output") != nil {
			if _, ok := cmd.GetFlagCompletionFunc("output"); !ok {
				missing = append(missing, cmd.CommandPath())
			}
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(root)

	if len(missing) > 0 {
		t.Errorf("commands with an --output flag but no completion for it: %v", missing)
	}
}
