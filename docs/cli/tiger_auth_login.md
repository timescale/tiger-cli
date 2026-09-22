## tiger auth login

Authenticate with Tiger Cloud API

### Synopsis

Authenticate with Tiger Cloud API using predefined keys or an interactive OAuth flow

By default, the command will launch an interactive OAuth flow in your browser to sign in.
The OAuth flow will:
- Open your browser for authentication
- Let you select a project (if you have multiple)
- Store an OAuth session for the selected project

If the browser cannot be opened, the command prints a short code to enter in a browser on
any other machine instead. Use --headless to go straight to that flow.

Use --project-id to pick the project up front and skip the interactive selection. After
logging in, you can switch projects with 'tiger project use'.

The credentials and project ID will be stored securely in the system keyring, or in a fallback file with
restricted permissions. Unless the login lands on the same project as the previous login, the default
service (config key service_id) is cleared, since it belongs to the project it was set in.

You may also provide API keys via flags or environment variables, in which case they will be used
directly. The CLI will prompt for any missing information.

You can find your API credentials at: https://console.cloud.tigerdata.com/dashboard/settings

```
tiger auth login [flags]
```

### Examples

```
  # Interactive login with OAuth (opens browser)
  tiger auth login

  # OAuth login without the interactive project selection
  tiger auth login --project-id my-project-id

  # Login from a machine the browser redirect cannot reach (SSH session, container)
  tiger auth login --headless

  # Login with keys (project ID will be auto-detected)
  tiger auth login --public-key your-public-key --secret-key your-secret-key

  # Login using environment variables
  export TIGER_PUBLIC_KEY="your-public-key"
  export TIGER_SECRET_KEY="your-secret-key"
  tiger auth login
```

### Options

```
      --headless            Authorize by entering a code in a browser on any machine, instead of waiting for a redirect back to this one
  -h, --help                help for login
      --project-id string   Project ID to log in to (skips interactive project selection)
      --public-key string   Public key for authentication
      --secret-key string   Secret key for authentication
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
