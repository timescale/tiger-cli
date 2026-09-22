## tiger feedback

Submit feedback or a bug report

### Synopsis

Submit feedback or a bug report to the Tiger Data team.

The message is sent with the email address of your account, so the team can
follow up, and with the CLI version and operating system. Pass the message as
an argument, or omit it to read from stdin.

This does not open a support case and returns no ticket to track. For anything
that needs a tracked response, open a support ticket in the Tiger Cloud
console; the command prints the link for your project.

```
tiger feedback [message] [flags]
```

### Examples

```
  # Submit feedback as an argument
  tiger feedback "I can't connect to my service after resuming it"

  # Submit feedback from stdin
  echo "Great tool!" | tiger feedback

  # Submit feedback interactively
  tiger feedback
  # → Enter your feedback (press Ctrl+D when done):
```

### Options

```
  -h, --help   help for feedback
```

### Options inherited from parent commands

```
      --analytics                 enable/disable usage analytics (default true)
      --color                     enable colored output (default true)
      --config-dir string         config directory (default "~/.config/tiger")
      --password-storage string   password storage method (keyring, pgpass, none) (default "keyring")
      --service-id string         service ID or name
      --version-check             check for updates on startup (default true)
```

### SEE ALSO

* [tiger](tiger.md)	 - Tiger CLI - Tiger Cloud Platform command-line interface
