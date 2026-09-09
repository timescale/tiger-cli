## tiger project use

Switch the active Tiger Cloud project

### Synopsis

Switch the Tiger Cloud project that subsequent commands operate on.

Switching requires an OAuth login ('tiger auth login' without API keys), because an API key
is scoped to a single project. To use an API key for another project, run 'tiger auth login'
with that project's keys instead.

The default service (config key service_id) belongs to the project it was set in, so it is
cleared when you switch away.

Example:
  tiger project use my-project-id

```
tiger project use <project-id> [flags]
```

### Options

```
  -h, --help   help for use
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
