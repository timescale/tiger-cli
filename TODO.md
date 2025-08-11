TODO items:
- we should get a Project ID from the API when we login (need to add a new endpoint to the API)
- need to add JWT/OATH2 support so that users don't handle API keys
- the api key stuff right now is messy. we require setting the api key from the public key and private key as public_key:private_key. should be a single string instead.


spec updates needed:
 - ✅ change service operations to use --wait-timeout instead of --timeout
 - ✅ implement --wait-timeout flag to accept time.ParseDuration format (e.g., "30m", "1h30m", "90s")
 - ✅ update --timeout flag for db test-connection to accept time.ParseDuration format 
 - ✅ implement exit code 2 for wait-timeout scenarios (operation continues on server)
 - update global exit code mapping per new spec (authentication moved to code 3, etc.)
 - ensure wait-timeout operations display status updates every 10 seconds while waiting
 - change from "tiger services" to "tiger service" with aliases for "services" and "svc"

small things:
 - ✅ implement flag not to save password to ~/.pgpass (replaced with --password-storage flag)
 - make sure create makes the service the default service (with option to not do this)
 - update create-service and update-password commands to respect global --password-storage flag (keyring|pgpass|none)