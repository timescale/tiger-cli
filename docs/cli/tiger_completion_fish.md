## tiger completion fish

Generate the autocompletion script for fish

### Synopsis

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	tiger completion fish | source

To load completions for every new session, execute once:

	tiger completion fish > ~/.config/fish/completions/tiger.fish

You will need to start a new shell for this setup to take effect.


```
tiger completion fish [flags]
```

### Options

```
  -h, --help              help for fish
      --no-descriptions   disable completion descriptions
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

* [tiger completion](tiger_completion.md)	 - Generate the autocompletion script for the specified shell
