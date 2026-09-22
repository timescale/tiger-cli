## tiger project list

List all projects

### Synopsis

List the Tiger Cloud projects you have access to.

The active project — the one subsequent commands operate on — is marked in the
output. Use 'tiger project use' to switch to another one.

```
tiger project list [flags]
```

### Options

```
  -h, --help            help for list
  -o, --output string   Output format (json, yaml, table)
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

* [tiger project](tiger_project.md)	 - Manage Tiger Cloud projects
