## tiger skills install

Install agent skills for an AI coding agent

### Synopsis

Install agent skills for an AI coding agent.

Skills are installed for the current user into ~/.agents/skills, which most
coding agents read, or into the client's own skills directory for clients that
don't.

Re-running the command updates the installed skills to the latest version and
removes any that are no longer available. Existing skills that weren't
installed by Tiger CLI are never replaced unless --force is given.

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

  # Install into a custom skills directory
  tiger skills install claude-code --skills-dir ~/my-skills
```

### Options

```
      --force               Replace existing skills that weren't installed by Tiger CLI
  -h, --help                help for install
      --skills-dir string   Custom skills directory to install into (overrides the client's default)
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
