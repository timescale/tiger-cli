## tiger service update-password

Update the master password for a service

### Synopsis

Update the master password for a specific database service.

The service ID can be provided as an argument or will use the default service
from your configuration. This command updates the master password for the
'tsdbadmin' user used to authenticate to the database service.

Examples:
  # Update password for default service
  tiger service update-password --new-password new-secure-password

  # Update password for specific service
  tiger service update-password svc-12345 --new-password new-secure-password

  # Update password using environment variable (TIGER_NEW_PASSWORD)
  export TIGER_NEW_PASSWORD="new-secure-password"
  tiger service update-password svc-12345

  # Update password and save to .pgpass (default behavior)
  tiger service update-password svc-12345 --new-password new-secure-password

  # Update password without saving (using global flag)
  tiger service update-password svc-12345 --new-password new-secure-password --password-storage none

```
tiger service update-password [service-id] [flags]
```

### Options

```
  -h, --help                  help for update-password
      --new-password string   New password for the tsdbadmin user (can also be set via TIGER_NEW_PASSWORD env var)
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

