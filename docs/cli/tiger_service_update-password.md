## tiger service update-password

Update the master password for a service

### Synopsis

Update the master password for a specific database service.

The service ID can be provided as an argument or will use the default service
from your configuration. This command updates the master password for the
'tsdbadmin' user used to authenticate to the database service.

A read replica ID is rejected — read replicas share the primary's credentials,
so update the password on the primary instead.

```
tiger service update-password [service-id] [flags]
```

### Examples

```
  # Update password for default service, interactively prompts
  tiger service update-password

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

  # Auto-generate a secure password
  tiger service update-password --auto-generate
```

### Options

```
      --auto-generate         Auto-generate a secure password
  -h, --help                  help for update-password
      --new-password string   New password for the tsdbadmin user (can also be set via TIGER_NEW_PASSWORD env var)
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

* [tiger service](tiger_service.md)	 - Manage database services
