## tiger auth login

Authenticate with TigerData API

### Synopsis

Authenticate with TigerData API using predefined keys or an interactive OAuth flow

By default, the command will launch an interactive OAuth flow in your browser to create new API keys.
The OAuth flow will:
- Open your browser for authentication
- Let you select a project (if you have multiple)
- Create API keys automatically for the selected project

The keys will be combined and stored securely in the system keyring or as a fallback file.
The project ID will be stored in the configuration file.

You may also provide API keys via flags or environment variables, in which case
they will be used directly. The CLI will prompt for any missing information.

You can find your API credentials and project ID at: https://console.cloud.timescale.com/dashboard/settings

Examples:
  # Interactive login with OAuth (opens browser, creates API keys automatically)
  tiger auth login

  # Login with project ID (will prompt for keys if not provided)
  tiger auth login --project-id your-project-id

  # Login with keys and project ID
  tiger auth login --public-key your-public-key --secret-key your-secret-key --project-id your-project-id

  # Login using environment variables
  export TIGER_PUBLIC_KEY="your-public-key"
  export TIGER_SECRET_KEY="your-secret-key"
  export TIGER_PROJECT_ID="proj-123"
  tiger auth login

```
tiger auth login [flags]
```

### Options

```
  -h, --help                help for login
      --project-id string   Default project ID to set in configuration
      --public-key string   Public key for authentication
      --secret-key string   Secret key for authentication
```

### Options inherited from parent commands

```
      --analytics                 enable/disable usage analytics (default true)
      --config-dir string         config directory (default "/Users/nathan/.config/tiger")
      --debug                     enable debug logging
  -o, --output string             output format (json, yaml, table)
      --password-storage string   password storage method (keyring, pgpass, none) (default "keyring")
      --service-id string         service ID
```

### SEE ALSO

* [tiger auth](tiger_auth.md)	 - Manage authentication and credentials

