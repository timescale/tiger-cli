## tiger skills install

Install agent skills for an AI coding agent

### Synopsis

Install agent skills for an AI coding agent.

Skills are downloaded from https://github.com/timescale/pg-aiguide and installed
for the current user into ~/.agents/skills, which most coding agents read. For
clients that read skills from their own directory, each skill is symlinked
into that directory, so every client shares a single copy.

Existing skills with the same names are replaced, so re-running the command
updates the installed skills to the latest version.

Supported Clients:
  claude-code              Claude Code (~/.claude/skills)
  cursor                   Cursor (~/.agents/skills)
  devin                    Devin (~/.agents/skills)
  codex                    Codex (~/.agents/skills)
  gemini                   Gemini CLI (~/.agents/skills)
  vscode                   VS Code (~/.agents/skills)
  antigravity              Google Antigravity (~/.gemini/antigravity/skills)
  kiro-cli                 Kiro CLI (~/.kiro/skills)
  copilot                  GitHub Copilot CLI (~/.agents/skills)

If no client is specified, you'll be prompted to select one interactively.

```
tiger skills install [client] [flags]
```

### Examples

```
  # Interactive client selection
  tiger skills install

  # Install for Claude Code
  tiger skills install claude-code

  # Install for Codex
  tiger skills install codex
```

### Options

```
  -h, --help   help for install
```

### Options inherited from parent commands

```
      --analytics                 enable/disable usage analytics (default true)
      --color                     enable colored output (default true)
      --config-dir string         config directory (default "~/.config/tiger")
      --password-storage string   password storage method (keyring, pgpass, none) (default "keyring")
      --service-id string         service ID
      --version-check             check for updates on startup (default true)
```

### SEE ALSO

* [tiger skills](tiger_skills.md)	 - Manage agent skills for AI coding agents
