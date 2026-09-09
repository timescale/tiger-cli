## tiger db create role

Create a new database role

### Synopsis

Create a new database role with optional read-only enforcement.

The service ID can be provided as an argument or will use the default service
from your configuration. A read replica ID is rejected, since replicas are
read-only; create the role on the primary instead.

By default, a secure random password is auto-generated for the new role. You can:
- Provide an explicit password with --password=<value>
- Use TIGER_NEW_PASSWORD environment variable
- Let it auto-generate (default)

The password is saved according to your --password-storage setting (keyring, pgpass, or none).

Read-Only Mode for AI Agents:
The --read-only flag enables permanent read-only enforcement at the PostgreSQL level
using the tsdb_admin.read_only_role extension setting. This is designed to provide
safe database access for AI agents and automated tools that need to read production
data without risk of modification.

Examples:
  # Create a role with global database access (uses default service, auto-generates password)
  tiger db create role --name ai_analyst --from tsdbadmin

  # Create a role for specific service
  tiger db create role svc-12345 --name ai_analyst

  # Create a read-only role
  tiger db create role --name ai_analyst --read-only

  # Create a read-only role with same grants as another role
  tiger db create role --name ai_analyst --read-only --from app_role

  # Create a read-only role inheriting from multiple roles
  tiger db create role --name ai_analyst --read-only --from app_role --from readonly_role

  # Create a read-only role with statement timeout
  tiger db create role --name ai_analyst --read-only --statement-timeout 30s

  # Create a role with specific password
  tiger db create role --name ai_analyst --password=my-secure-password

  # Create a role with password from environment variable
  TIGER_NEW_PASSWORD=my-secure-password tiger db create role --name ai_analyst

Technical Details:
This command executes PostgreSQL statements in a transaction to create and configure the role.

CREATE ROLE Options Used:
  - LOGIN: Always enabled to allow the role to connect
  - PASSWORD: Always set (from flag, env var, or auto-generated)
  - IN ROLE: Added when --from flag is provided to inherit grants from existing roles

PostgreSQL Configuration Parameters That May Be Set:
  - tsdb_admin.read_only_role: Set to 'true' when --read-only flag is used
    (enforces permanent read-only mode for the role)
  - statement_timeout: Set when --statement-timeout flag is provided
    (kills queries that exceed the specified duration, in milliseconds)

```
tiger db create role [service-id] [flags]
```

### Options

```
      --from strings                 Roles to inherit grants from (e.g., --from app_role --from readonly_role or --from app_role,readonly_role)
  -h, --help                         help for role
      --name string                  Role name to create (required)
  -o, --output string                output format (json, yaml, table)
      --password string              Password for the role. If not provided, checks TIGER_NEW_PASSWORD environment variable, otherwise auto-generates a secure random password.
      --read-only                    Enable permanent read-only enforcement via tsdb_admin.read_only_role
      --statement-timeout duration   Set statement timeout for the role (e.g., 30s, 5m)
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

* [tiger db create](tiger_db_create.md)	 - Create database resources
