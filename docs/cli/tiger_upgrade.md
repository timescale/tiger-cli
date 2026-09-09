## tiger upgrade

Upgrade the Tiger CLI to the latest version

### Synopsis

Download and install the latest published version of the Tiger CLI, replacing the currently running binary.

If Tiger CLI was installed via a package manager (Homebrew, apt, yum/dnf), the upgrade will be refused with a suggestion to use that package manager instead.

```
tiger upgrade [flags]
```

### Options

```
  -h, --help   help for upgrade
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

* [tiger](tiger.md)	 - Tiger CLI - Tiger Cloud Platform command-line interface
