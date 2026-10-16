# Docker build cache maintenance

The root administrator can inspect Docker Engine build cache and clear unused
cache from **System settings → Operations → Performance**. These operations run
through `new-api-updater` on its Docker host. The application container does not
need the Docker socket or Docker CLI.

The application and updater images must both include this feature. An older or
unavailable updater produces an unavailable state, rather than a zero cache size.
The existing `UPDATE_SIDECAR_URL` setting selects the updater. Standard Compose
already provides its private network and `/var/run/docker.sock` mount; keep port
18090 private because the updater is a privileged internal service. Deploy ready
registry images by explicit release tag, following the backup and deployment
rules in [../AGENTS.md](../AGENTS.md).

The filesystem usage bar describes the filesystem containing the application's
cache directory. It is not the size of that directory or a Docker usage breakdown.
Docker cache usage is reported separately and must not be added to that bar.

Cleanup uses Docker Engine's build cache prune with `all=true` and `until=24h`.
Only unused build cache older than 24 hours is eligible. It does not prune images,
containers, networks, or volumes. Future builds may take longer. The reclaimable
size is an estimate that excludes in-use and shared records; the age filter and
Docker's reference accounting may reduce the space actually reclaimed. The result
shows the bytes Docker reports as reclaimed.

The public API exposes root-only `GET /api/performance/docker_cache` and
`DELETE /api/performance/docker_cache`. Cleanup requires confirmation in the UI,
is audited as `performance.clear_docker_cache`, and cannot run alongside an
application update or another cache cleanup. The updater exposes only fixed
Docker operations for this feature; the browser cannot supply shell commands or
Docker API paths. Reverse proxies must allow at least 150 seconds for cleanup.

This feature covers the connected Docker Engine builder. Separate Buildx builders
using the `docker-container`, remote, or Kubernetes drivers have their own cache
stores and are not included. Image layers, container logs, and volume data also
need separate accounting; a small build cache does not imply low Docker disk use.
