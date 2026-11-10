## tiger completion bash

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(tiger completion bash)

To load completions for every new session, execute once:

#### Linux:

	tiger completion bash > /etc/bash_completion.d/tiger

#### macOS:

	tiger completion bash > $(brew --prefix)/etc/bash_completion.d/tiger

You will need to start a new shell for this setup to take effect.


```
tiger completion bash
```

### Options

```
  -h, --help              help for bash
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
