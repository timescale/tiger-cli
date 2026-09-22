## tiger config list

List current configuration

### Synopsis

List the current CLI configuration settings

```
tiger config list [flags]
```

### Options

```
  -h, --help            help for list
      --no-defaults     do not show default values for unset fields
  -o, --output string   output format (json, yaml, table)
      --with-env        apply environment variable overrides
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

* [tiger config](tiger_config.md)	 - Manage CLI configuration
