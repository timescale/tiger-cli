## tiger service delete

Delete a database service

### Synopsis

Delete a database service permanently.

This operation is irreversible. By default, you will be prompted to type the service ID
to confirm deletion, unless you use the --confirm flag.

Note for AI agents: Always confirm with the user before performing this destructive operation.

Examples:
  # Delete a service (with confirmation prompt)
  tiger service delete svc-12345

  # Delete service without confirmation prompt
  tiger service delete svc-12345 --confirm

  # Delete service without waiting for completion
  tiger service delete svc-12345 --no-wait

  # Delete service with custom wait timeout
  tiger service delete svc-12345 --wait-timeout 15m

```
tiger service delete [service-id] [flags]
```

### Options

```
      --confirm                 Skip confirmation prompt (AI agents must confirm with user first)
  -h, --help                    help for delete
      --no-wait                 Don't wait for deletion to complete, return immediately
      --wait-timeout duration   Wait timeout duration (e.g., 30m, 1h30m, 90s) (default 30m0s)
```

### Options inherited from parent commands

```
      --analytics                 enable/disable usage analytics (default true)
      --config-dir string         config directory (default "/Users/nathan/.config/tiger")
      --debug                     enable debug logging
  -o, --output string             output format (json, yaml, table)
      --password-storage string   password storage method (keyring, pgpass, none) (default "keyring")
      --project-id string         project ID
      --service-id string         service ID
```

### SEE ALSO

* [tiger service](tiger_service.md)	 - Manage database services

