## tiger auth logout

Remove stored credentials

### Synopsis

Remove stored credentials. For OAuth logins, also revokes the refresh token server-side.

```
tiger auth logout [flags]
```

### Options

```
  -h, --help   help for logout
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

* [tiger auth](tiger_auth.md)	 - Manage authentication and credentials
