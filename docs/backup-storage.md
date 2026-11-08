# Database backup storage

The updater creates and verifies a PostgreSQL custom-format dump before each
application update. It stores the dump in `${UPDATER_COMPOSE_DIR}/backups/` as
`new-api-before-update-<UTC timestamp>.dump`. On the current server this resolves
to `/root/repos/new-api/backups/`.

The updater keeps these copies on the same server as the application. It does
not upload or copy them to a remote storage platform. A server disk failure can
therefore affect both the application and its local backups; configure and
verify an off-server copy separately before treating the backups as disaster
recovery.

System settings → Operations → Performance reports the folder's usage and can
remove entries older than 30 days. It always preserves the two newest entries.
The operation skips symbolic links and never traverses another filesystem. It
does not access PostgreSQL's active Docker volume or other working data. Docker
image cleanup is separate and preserves images used by any container; Docker
volumes are not pruned.
