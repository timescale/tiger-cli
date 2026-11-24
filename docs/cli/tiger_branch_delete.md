## tiger branch delete

Delete a branch

### Synopsis

Delete a branch permanently, along with any changes made on it. The parent
service is not affected.

The branch can be given by ID or name.

This operation is irreversible. By default, you will be prompted to type the
branch ID to confirm deletion — the ID, not the name — unless you use the
--confirm flag.

Note for AI agents: Always confirm with the user before performing this destructive operation.

```
tiger branch delete <name-or-id> [flags]
```

### Examples

```
  # Delete a branch (with confirmation prompt)
  tiger branch delete br-12345

  # Delete a branch without confirmation prompt
  tiger branch delete br-12345 --confirm
```

### Options

```
      --confirm   Skip confirmation prompt (AI agents must confirm with user first)
  -h, --help      help for delete
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
