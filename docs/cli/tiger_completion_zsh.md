## tiger completion zsh

Generate the autocompletion script for zsh

### Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(tiger completion zsh)

To load completions for every new session, execute once:

#### Linux:

	tiger completion zsh > "${fpath[1]}/_tiger"

#### macOS:

	tiger completion zsh > $(brew --prefix)/share/zsh/site-functions/_tiger

You will need to start a new shell for this setup to take effect.


```
tiger completion zsh [flags]
```

### Options

```
  -h, --help              help for zsh
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
