# Tiger CLI Future Features

This document contains specifications for features that should be considered for the future. Lots of these are Claude generated. Keeping them here for reference.

## Project Management (Future)

**Note: Current API keys are scoped to a single project. Multi-project support will be added once API keys support multiple projects.**

#### `tiger projects`
Manage TigerData projects.

**Subcommands:**
- `list`: List all projects
- `show`: Show project details
- `set-default`: Set default project

**Examples:**
```bash
# List projects
tiger projects list

# Show project details
tiger projects show proj-12345

# Set default project
tiger projects set-default proj-12345
```

**Options:**
- None (project creation/deletion will be added in future releases)

---

### Project Creation and Deletion

#### `tiger projects create`
Create a new TigerData project.

**Arguments:**
- `--name`: Project name (required)
- `--description`: Project description (optional)

**Examples:**
```bash
# Create project
tiger projects create --name "My Project" --description "Production database"
```

**Environment Variables:**
- `TIGER_DEFAULT_REGION`: Default region for new projects

---

#### `tiger projects delete`
Delete a TigerData project.

**Arguments:**
- `project-id`: Project ID to delete (required)

**Options:**
- `--confirm`: Skip confirmation prompt
- `--force`: Force deletion even if project contains resources

**Examples:**
```bash
# Delete project with confirmation
tiger projects delete proj-12345

# Force delete without confirmation
tiger projects delete proj-12345 --confirm --force
```

**Safety Features:**
- Requires explicit confirmation unless `--confirm` is used
- Prevents deletion of projects with active resources unless `--force` is used
- Provides detailed warning about resources that will be deleted

**Error Conditions:**
- Project not found
- Project contains active resources (without `--force`)
- Insufficient permissions to delete project
- Project is the user's last remaining project

---

## Database Operations (Future)

### Tiger DB Extended Commands

#### `tiger db dump`
Export database contents to a file.

**Arguments:**
- `service-id`: Service ID to dump from (required)

**Options:**
- `--output`: Output file for dumps
- `--format`: Dump format (sql, custom, tar)

**Examples:**
```bash
# Dump database
tiger db dump svc-12345 --output backup.sql
tiger db dump svc-12345 --output backup.dump --format custom
```

---

#### `tiger db restore`
Import database from a dump file.

**Arguments:**
- `service-id`: Service ID to restore to (required)

**Options:**
- `--input`: Input file for restore
- `--format`: Dump format (sql, custom, tar)

**Examples:**
```bash
# Restore from dump
tiger db restore svc-12345 --input backup.sql
```

---

#### `tiger db reset`
Reset database to clean state.

**Arguments:**
- `service-id`: Service ID to reset (required)

**Options:**
- `--confirm`: Confirm destructive operations

**Examples:**
```bash
# Reset database
tiger db reset svc-12345 --confirm
```

---

#### `tiger db migrate`
Run database migrations.

**Arguments:**
- `service-id`: Service ID to migrate (required)

**Examples:**
```bash
# Run migrations
tiger db migrate svc-12345
```

---

#### `tiger db seed`
Seed database with initial data.

**Arguments:**
- `service-id`: Service ID to seed (required)

**Examples:**
```bash
# Seed database
tiger db seed svc-12345
```

---

## Migration Management (Future)

#### `tiger migrations`
Manage database schema migrations.

**Subcommands:**
- `list`: List migration history
- `new`: Create a new migration file
- `up`: Apply pending migrations
- `down`: Rollback migrations
- `status`: Show migration status
- `validate`: Validate migration files

**Examples:**
```bash
# List migrations
tiger migrations list svc-12345

# Create new migration
tiger migrations new --name "add_users_table"

# Apply migrations
tiger migrations up svc-12345

# Rollback last migration
tiger migrations down svc-12345 --steps 1

# Show status
tiger migrations status svc-12345
```

**Options:**
- `--name`: Migration name
- `--steps`: Number of migration steps
- `--target`: Target migration version

---

## Monitoring & Diagnostics (Future)

#### `tiger inspect`
Database performance and diagnostic tools.

**Subcommands:**
- `bloat`: Check table bloat
- `locks`: Show database locks
- `queries`: Analyze running queries
- `slow-queries`: Find slow queries
- `stats`: Show database statistics
- `vacuum`: Show vacuum statistics

**Examples:**
```bash
# Check table bloat
tiger inspect bloat svc-12345

# Show active locks
tiger inspect locks svc-12345

# Analyze slow queries
tiger inspect slow-queries svc-12345 --limit 10
```

**Options:**
- `--limit`: Limit number of results
- `--threshold`: Performance threshold
- `--format`: Output format

---

## Operations & Logs (Future)

#### `tiger operations`
View and manage long-running operations.

**Subcommands:**
- `list`: List operations
- `show`: Show operation details
- `cancel`: Cancel operation
- `logs`: View operation logs

**Examples:**
```bash
# List operations
tiger operations list

# Show operation details
tiger operations show op-12345

# View logs
tiger operations logs op-12345 --follow
```

**Options:**
- `--follow`: Follow log output
- `--since`: Show logs since timestamp
- `--status`: Filter by operation status

---

## Additional Future Features

### Advanced Monitoring
- Real-time performance dashboards
- Alerting and notification management
- Custom metric collection
- Historical performance analysis

### Backup Management
- Automated backup scheduling
- Point-in-time recovery
- Cross-region backup replication
- Backup retention policies

### Security Features
- Advanced authentication methods (SSO, MFA)
- Role-based access control (RBAC)
- Audit logging
- Encryption key management

### Integration Features
- CI/CD pipeline integration
- Infrastructure as Code (Terraform) support
- Monitoring tool integrations (Prometheus, Grafana)
- Log shipping to external systems
