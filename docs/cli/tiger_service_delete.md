## tiger service delete

Delete a database service

### Synopsis

Delete a database service permanently.

The service can be given by ID or name, but must be given explicitly: there is
no fallback to the default service.

This operation is irreversible. By default, you will be prompted to type the
service ID to confirm deletion — the ID, not the name — unless you use the
--confirm flag.

Note for AI agents: Always confirm with the user before performing this destructive operation.

```
tiger service delete [name-or-id] [flags]
```

### Examples

```
  # Delete a service (with confirmation prompt)
  tiger service delete svc-12345

  # Delete service without confirmation prompt
  tiger service delete svc-12345 --confirm
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
      --service-id string         service ID or name
      --version-check             check for updates on startup (default true)
```

### SEE ALSO

* [tiger service](tiger_service.md)	 - Manage database services
