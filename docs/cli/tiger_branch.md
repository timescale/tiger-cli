## tiger branch

Manage branches of database services

### Synopsis

Manage branches of Fluid Services.

A branch is a lightweight, ephemeral copy of a Fluid Service, for development,
testing, CI/CD, and agent workflows. It shares its parent's storage as of the
moment it was created and gets its own compute.

### Options

```
  -h, --help   help for branch
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

* [tiger](tiger.md)	 - Tiger CLI - Tiger Cloud Platform command-line interface
* [tiger branch create](tiger_branch_create.md)	 - Create a branch of a database service
* [tiger branch delete](tiger_branch_delete.md)	 - Delete a branch
* [tiger branch get](tiger_branch_get.md)	 - Show detailed information about a branch
* [tiger branch list](tiger_branch_list.md)	 - List the branches of a service
* [tiger branch logs](tiger_branch_logs.md)	 - View logs for a branch
* [tiger branch rename](tiger_branch_rename.md)	 - Rename a branch
* [tiger branch resize](tiger_branch_resize.md)	 - Resize a branch
* [tiger branch start](tiger_branch_start.md)	 - Start a stopped branch
* [tiger branch stop](tiger_branch_stop.md)	 - Stop a running branch
* [tiger branch update-password](tiger_branch_update-password.md)	 - Update the master password for a branch
