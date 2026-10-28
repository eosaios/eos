# EOS local credentials

On macOS, `Get`, `Set` and `Delete` use the same vault format as `eos-core-keyring`.
The shared directory is `~/Library/Application Support/eos/credentials`.
Service and account identifiers retain product and environment isolation.

AES-256-GCM values use random nonces. New v2 records authenticate both service and
account; existing Agent v1 records remain readable. The local random key file and
vault are private to the OS user (0600, directory 0700). This does not protect
against malicious processes running as that same user or copying both files.

All read/modify/write operations hold a cross-process lock, persist a private
temporary file and replace the vault atomically. Corruption and missing keys fail
without overwriting existing data. Logout persists a tombstone so old native
credentials cannot resurrect a session.

Legacy Keychain import is read-only and disables authentication UI through
`LAContext.interactionNotAllowed`. A protected old entry requires signing in again
in the owning app. New writes and deletes do not access Keychain. Windows and
Linux retain the native backend in this macOS rollout.
