# Tiger CLI OAuth Integration Specification

## Overview

This document outlines the OAuth 2.0 integration for Tiger CLI, providing secure
authentication with the TigerData Cloud Platform API. The implementation
supports both interactive user authentication and programmatic authentication
for CI/CD environments.

## Authentication Flows

### 1. Interactive Login Flow (`tiger auth login`)

The interactive flow uses OAuth 2.0 Authorization Code flow with PKCE (Proof Key
for Code Exchange) for secure browser-based authentication.

#### Flow Steps:

1. **CLI Initiation**
   - User runs `tiger auth login`
   - CLI generates PKCE code verifier and challenge
   - CLI starts local HTTP server on random available port (e.g.,
     `http://localhost:8080/callback`)
   - CLI constructs authorization URL with:
     - `client_id`: Tiger CLI application ID registered in FusionAuth
     - `redirect_uri`: Local callback URL
     - `response_type=code`
     - `scope`: Required API scopes (e.g., `api:read api:write`)
     - `code_challenge`: PKCE challenge
     - `code_challenge_method=S256`
     - `state`: Random state parameter for CSRF protection

2. **Browser Redirect**
   - CLI opens user's default browser to authorization URL (new page in web-cloud)
   - User completes authentication flow in browser
     - Must log in if not already logged in
     - Click a button to authorize the CLI app
   - On success: Browser redirects to local callback with authorization code
   - On decline: Browser redirects to local callback with error parameters
   - On error: Browser redirects to local callback with error parameters

3. **Authorization Code Exchange**
   - CLI receives callback request with authorization code
   - CLI displays success page in browser: "Authentication successful! You can
     now return to your terminal."
   - CLI exchanges authorization code for tokens via POST to token endpoint:
     - `grant_type=authorization_code`
     - `client_id`: Tiger CLI application ID
     - `code`: Authorization code from callback
     - `redirect_uri`: Same URI used in authorization request
     - `code_verifier`: PKCE code verifier

4. **Token Storage**
   - CLI receives access token, refresh token, and token metadata
   - Tokens stored securely in system keychain (macOS Keychain, Windows
     Credential Manager, Linux Secret Service)
   - Fallback to encrypted file storage if keychain unavailable
   - CLI shuts down local HTTP server

#### Error Handling:
- Network errors: Retry with exponential backoff
- User cancellation: Clean exit with helpful message
- Invalid credentials: Clear error message with retry option
- PKCE validation failures: Security error, force re-authentication

### 2. Client Credentials Flow (CI/CD)

For non-interactive environments, Tiger CLI supports OAuth 2.0 Client
Credentials flow using pre-configured client credentials.

#### Flow Steps:

1. **Credential Configuration**
   - User provides `CLIENT_ID` and `CLIENT_SECRET` via:
     - Environment variables: `TIGER_CLIENT_ID`, `TIGER_CLIENT_SECRET`
     - CLI flags: `--client-id`, `--client-secret`
     - Configuration file (not recommended for secrets)

2. **Token Exchange**
   - CLI directly exchanges credentials for access token via POST to token
     endpoint:
     - `grant_type=client_credentials`
     - `client_id`: Provided client ID
     - `client_secret`: Provided client secret
     - `scope`: Required API scopes

3. **Token Usage**
   - Tokens used immediately, not persisted to keychain or filesystem
   - Fresh tokens obtained for each CLI invocation
   - No refresh token issued (client credentials flow doesn't use refresh
     tokens)

#### Security Considerations:
- Client secrets should be managed as secure environment variables
- Tokens have shorter expiration times compared to interactive flow
- No persistent token storage reduces security risk

## Token Management

### Token Storage

#### Secure Keychain (Preferred)
- **macOS**: Keychain Services API
- **Windows**: Windows Credential Manager
- **Linux**: Secret Service (libsecret/gnome-keyring)

Storage keys:
- `tiger-cli-access-token`: Current access token
- `tiger-cli-refresh-token`: Current refresh token
- `tiger-cli-token-metadata`: Token expiration and scope information

#### File-based Fallback If keychain is unavailable, tokens stored in local
file:
- Location: `~/.config/tiger/credentials.json`
- Permissions: 0600 (owner read/write only)

### Token Usage

All API requests include bearer token in Authorization header: ```
Authorization: Bearer <access_token> ```

### Token Refresh Flow

1. **Automatic Refresh**
   - CLI checks token expiration before each API request
   - If token expires within 5 minutes, trigger refresh
   - Use refresh token to obtain new access token

2. **Refresh Request**
   - POST to token endpoint:
     - `grant_type=refresh_token`
     - `client_id`: Tiger CLI application ID
     - `refresh_token`: Current refresh token

3. **Refresh Response**
   - New access token and optionally new refresh token
   - Update stored tokens with new values
   - Retry original API request with new access token

4. **Refresh Failure**
   - If refresh token expired/invalid: Clear stored tokens
   - Prompt user to re-authenticate: "Your session has expired. Please run
     'tiger auth login' to re-authenticate."
   - For client credentials flow: Obtain fresh token using credentials

## FusionAuth Integration

### Endpoints

#### Authorization Endpoint ``` GET
https://auth.console.cloud.timescale.com/oauth2/authorize ```

Parameters:
- `client_id`: Tiger CLI application ID
- `redirect_uri`: Callback URL
- `response_type=code`: Authorization code flow
- `scope`: Space-separated scopes
- `code_challenge`: PKCE challenge
- `code_challenge_method=S256`: PKCE method
- `state`: CSRF protection token

#### Token Endpoint ``` POST
https://auth.console.cloud.timescale.com/oauth2/token Content-Type:
application/x-www-form-urlencoded ```

**Authorization Code Exchange:** ``` grant_type=authorization_code
&client_id={client_id} &code={authorization_code} &redirect_uri={redirect_uri}
&code_verifier={pkce_verifier} ```

**Client Credentials:** ``` grant_type=client_credentials &client_id={client_id}
&client_secret={client_secret} &scope={scopes} ```

**Refresh Token:** ``` grant_type=refresh_token &client_id={client_id}
&refresh_token={refresh_token} ```

#### Token Response Format ```json { "access_token":
"eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9...", "refresh_token":
"eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9...", "token_type": "Bearer", "expires_in":
3600, "scope": "api:read api:write" } ```

#### Signature RSA-256 signature using FusionAuth's private key, verifiable with
public key from JWKS endpoint: ``` GET
https://auth.console.cloud.timescale.com/.well-known/jwks.json ```

## CLI Command Structure

### Auth Commands

```bash # Interactive login tiger auth login

# Logout (clear stored tokens) tiger auth logout ```

### Global Authentication Flags

Available on all commands:
- `--client-id <id>`: Override default client ID
- `--client-secret <secret>`: Provide client secret for CI/CD flow
- `--no-keychain`: Force file-based token storage
