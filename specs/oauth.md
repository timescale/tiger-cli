# Tiger CLI OAuth Integration Specification

## Overview

This document outlines the OAuth 2.0 integration for Tiger CLI, providing secure
authentication with the TigerData Cloud Platform API. The implementation
supports both interactive user authentication and programmatic authentication
for CI/CD environments.

## Design Decisions

### OAuth flow uses new `web-cloud` authorization page

- Not the built-in FusionAuth login page
- Ensures user does not have to log in again if they're already logged into the console
- Page styling and functionality are fully in our control
- No need to deal with creating different FusionAuth applications, etc.
  (FusionAuth is not really well-suited to this use case).

### Client secret is not verified

- The client secret (per the OAuth standard) is not used or verified by the back-end
- There would be no way to truly secure it in a CLI tool distributed to end users
- State parameter and PKCE at least ensure that our CLI tool is both kicking off
  the OAuth flow and completing it
- Risk: malicious code running on the user's machine could perform the OAuth
  flow just like our CLI tool does
  - Could potentially steal credentials
  - User would still have to click "Authorize" to complete the flow
  - If user machine is compromised to that extent (running untrusted code),
    probably have bigger problems (e.g. could likely steal many other
    credentials directly from the filesystem)

### Client Credentials (PAT tokens) used for API authentication

- The OAuth access token is only used temporarily to create new client credentials (PAT token)
- The OAuth access token is not stored on the user's machine
- Public key and secret key (from client credentials) are concatenated, stored, and used as the API key
- Benefits:
  - Client credentials can be viewed in the UI and revoked (whereas OAuth access/refresh tokens cannot be)
  - No need to acquire a long-lived OAuth refresh token
  - No need to deal with refreshing access tokens (which expire often)
  - API keys are limited to just a single project (also potentially a downside)

### Existing GraphQL endpoints are used in OAuth flow

- GraphQL endpoints are used to:
  - Fetch user profile info
  - Fetch list of user's projects
  - Create new client credentials (PAT token)
- Endpoints already exist for the sake of the `web-cloud` front-end
- Endpoints already accept the OAuth access token for authentication
- No need to add corresponding endpoints to `savannah-public` that accept OAuth
  access tokens (unlike the other endpoints)

## Authentication Flows

### 1. Interactive OAuth Login Flow (`tiger auth login`)

The interactive flow uses OAuth 2.0 Authorization Code flow with PKCE (Proof Key
for Code Exchange) for secure browser-based authentication.

#### Flow Steps:

1. **CLI Initiation**
   - User runs `tiger auth login`
   - CLI generates PKCE code verifier and challenge
   - CLI starts local HTTP server on random available port (e.g. `http://localhost:8080/callback`)
   - CLI constructs authorization URL with:
     - `client_id`: Tiger CLI application ID
     - `redirect_uri`: Local callback URL
     - `response_type=code`
     - `scope`: Required API scopes (NOTE: not currently implemented)
     - `code_challenge`: PKCE challenge
     - `code_challenge_method=S256`
     - `state`: Random state parameter for CSRF protection

2. **Browser Redirect**
   - CLI opens user's default browser to authorization URL (e.g. https://console.cloud.timescale.com/oauth/authorize)
   - User completes authentication flow in browser
     - Must log in if not already logged in
     - Click "Authorize" button to authorize the CLI app
   - On success: Browser redirects to local callback with authorization code and state parameter
   - On error: Browser displays error message on authorization page

3. **Authorization Code Exchange**
   - CLI receives callback request with authorization code
   - CLI verifies state parameter is correct
   - CLI exchanges authorization code for access token via POST to token endpoint (e.g. https://console.cloud.timescale.com/api/idp/external/cli/token):
     - `grant_type=authorization_code`
     - `client_id`: Tiger CLI application ID
     - `code`: Authorization code from callback
     - `redirect_uri`: Same URI used in authorization request
     - `code_verifier`: PKCE code verifier
   - CLI redirects browser from local callback URL to success page (e.g. https://console.cloud.timescale.com/oauth/code/success)
   - CLI shuts down local HTTP server

4. **API Key Generation**
   - CLI uses access token to fetch list of user projects via GraphQL endpoint
   - User is prompted to choose a project (if more than one)
   - CLI uses access token to fetch user profile info
   - CLI uses access token to create new client credentials (PAT token)
     - Token is named `Tiger CLI - ${username}`

5. **Credential Storage**
   - Public key and secret key (client credentials) are concatenated with a
     colon to form the API key (e.g. `publicKey:secretKey`)
   - API key is stored securely in system keychain (macOS Keychain, Windows
     Credential Manager, Linux Secret Service)
   - Fallback to encrypted file storage if keychain unavailable
   - Project ID stored in config file

#### Error Handling:

- User cancellation: Clean exit with helpful message
- Invalid credentials: Clear error message with retry option
- PKCE validation failures: Security error, force re-authentication

### 2. Programmatic Login Flow (CI/CD)

For non-interactive environments, Tiger CLI supports providing credentials
manually via flags or environment variables. The CLI will prompt for credentials
that are missing.

#### Flow Steps:

1. **Credential Configuration**
   - User provides one or more of the following:
     - `--public-key` flag or `TIGER_PUBLIC_KEY` env var
     - `--secret-key` flag or `TIGER_SECRET_KEY` env var
     - `--project-id` flag or `TIGER_PROJECT_ID` env var
   - Flags take precedence over environment variables

2. **Prompt for missing credentials**
   - CLI prompts user for any values (public key, secret key, or project ID) that weren't provided
   - TODO: Should be possible to determine the project ID programmatically from
     the public key/secret key via an API call, which would render the project
     ID flag/env var unnecessary

3. **Credential Storage**
   - Public key and secret key (client credentials) are concatenated with a
     colon to form the API key (e.g. `publicKey:secretKey`)
   - API key is stored securely in system keychain (macOS Keychain, Windows
     Credential Manager, Linux Secret Service)
   - Fallback to encrypted file storage if keychain unavailable
   - Project ID stored in config file

#### Error Handling:

- User cancellation: Clean exit with helpful message
- Invalid credentials: Clear error message
- Failure to provide credentials when prompted: clear error message
