## tiger branch update-password

Update the master password for a branch

### Synopsis

Update the master password for the 'tsdbadmin' user of a branch.

Changing a branch's password doesn't change its parent service's.

The branch can be given by ID or name.

```
tiger branch update-password <name-or-id> [flags]
```

### Examples

```
  # Update a branch's password, interactively prompts
  tiger branch update-password br-12345

  # Update a specific branch's password
  tiger branch update-password br-12345 --new-password new-secure-password

  # Update password using environment variable (TIGER_NEW_PASSWORD)
  export TIGER_NEW_PASSWORD="new-secure-password"
  tiger branch update-password br-12345

  # Auto-generate a secure password
  tiger branch update-password br-12345 --auto-generate
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

* [tiger branch](tiger_branch.md)	 - Manage branches of database services
