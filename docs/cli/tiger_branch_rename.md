## tiger branch rename

Rename a branch

### Synopsis

Rename a branch.

Only the branch's name changes: its ID, endpoint, and data are untouched, so
existing connections and connection strings keep working.

Both the branch and the new name are required. The branch can be given by ID
or by its current name.

```
tiger branch rename <name-or-id> <new-name> [flags]
```

### Examples

```
  # Rename a branch
  tiger branch rename br-12345 feature/rbac
```

### Options

```
  -h, --help   help for rename
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

* [tiger branch](tiger_branch.md)	 - Manage branches of database services
