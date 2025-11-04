# Tiger CLI OAuth Authentication Features

**Note: These authentication flows can only be implemented once we have OAuth login support with JWT tokens.**

## Authentication (OAuth-based)

#### `tiger auth`
Manage authentication and credentials with OAuth support.

**Subcommands:**
- `login`: Authenticate with Tiger Cloud (OAuth flow)
- `logout`: Remove stored credentials
- `status`: Show current user information
- `token`: Manage API tokens

**Examples:**
```bash
# Interactive OAuth login (opens browser)
tiger auth login

# Web-based OAuth authentication
tiger auth login --web

# Show current token status
tiger auth status

# Logout and clear credentials
tiger auth logout
```

**OAuth Flow:**
1. `tiger auth login` opens browser to Tiger Cloud OAuth page
2. User authenticates and grants permissions
3. CLI receives JWT token and stores it securely
4. Subsequent commands use the stored JWT token for authentication

**Options:**
- `--web`: Use web browser for OAuth authentication (default)
- `--device-code`: Use device code flow for headless environments

**Token Storage:**
- JWT tokens stored securely in system keychain/credential manager
- Automatic token refresh when possible
- Fallback to re-authentication when tokens expire