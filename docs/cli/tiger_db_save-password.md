## tiger db save-password

Save password for a database service

### Synopsis

Save a password for a database service to configured password storage.

The service ID can be provided as an argument or will use the default service
from your configuration. The password can be provided via:
1. --password flag with explicit value (highest precedence)
2. TIGER_NEW_PASSWORD environment variable
3. Interactive prompt (if neither provided)

The password will be saved according to your --password-storage setting
(keyring, pgpass, or none).

```
tiger db save-password [service-id] [flags]
```

### Examples

```
  # Save password with explicit value (highest precedence)
  tiger db save-password svc-12345 --password=your-password

  # Using environment variable
  export TIGER_NEW_PASSWORD=your-password
  tiger db save-password svc-12345

  # Interactive password prompt (when neither flag nor env var provided)
  tiger db save-password svc-12345

  # Save password for custom role
  tiger db save-password svc-12345 --password=your-password --role readonly

  # Save to specific storage location
  tiger db save-password svc-12345 --password=your-password --password-storage pgpass
```

### Options

```
  -h, --help              help for save-password
  -p, --password string   Password to save
      --role string       Database role/username (default "tsdbadmin")
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

* [tiger db](tiger_db.md)	 - Database operations and management
