## tiger skills install

Install agent skills for AI coding agents

### Synopsis

Install agent skills for AI coding agents.

Skills are installed for the current user into one or more install locations:
the universal ~/.agents/skills directory, which most coding agents read, and
the skills directories of clients that don't read it. Each argument selects a
location, either by name or by the name of a client that reads it.

Install locations:
  universal                ~/.agents/skills (Cursor, Devin, Codex, Gemini CLI, VS Code, GitHub Copilot CLI)
  claude-code              ~/.claude/skills (Claude Code)
  antigravity              ~/.gemini/config/skills (Google Antigravity)
  kiro-cli                 ~/.kiro/skills (Kiro CLI)

With no arguments, you're prompted to select locations interactively, starting
from the locations Tiger CLI has installed skills to before.

Re-running the command updates the installed skills to the latest version and
removes any that are no longer available. Existing skills that weren't
installed by Tiger CLI are never replaced unless --force is given.

```
tiger skills install [client...] [flags]
```

### Examples

```
  # Interactive selection
  tiger skills install

  # Install to ~/.agents/skills and for Claude Code
  tiger skills install universal claude-code

  # Install into a custom skills directory
  tiger skills install --skills-dir ~/my-skills
```

### Options

```
      --force               Replace existing skills that weren't installed by Tiger CLI
  -h, --help                help for install
      --skills-dir string   Install into this skills directory only (not remembered for later installs)
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
