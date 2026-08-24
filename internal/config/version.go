package config

// These variables are set at build time via ldflags in the GoReleaser pipeline
// for production releases. Default values are used for local development builds.
var Version = "dev"
var BuildTime = "unknown"
var GitCommit = "unknown"
