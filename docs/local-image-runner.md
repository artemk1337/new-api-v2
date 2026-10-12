# Local Docker image runner

Release Docker images can be built by a repository-scoped GitHub Actions runner
on an Apple Silicon Mac with Docker Desktop running. Register the official
macOS ARM64 runner with the custom label `new-api-build` and start its user service.
The runner must have `docker`, `git`, and the usual shell tools on its PATH.

Set repository variable `NEW_API_USE_LOCAL_RUNNER=true` to route tagged Docker
builds and manifest publication to this runner. Delete the variable or set it to
`false` to restore hosted runners. For a single manual build, select
`local_runner=true` and an existing release tag in the Docker workflow dispatch.
Manual dispatch uses the workflow definition on `main`, so it can build tags
created before local runner support was added.

Both Linux architectures are built sequentially on a single runner; AMD64 uses
Docker Desktop emulation. Images, SBOMs, provenance, signatures, and multi-arch
manifests continue to be published to GHCR. Local mode skips GitHub Actions cache
exports and Docker build-record artifact uploads. Other workflows retain their
existing runners and storage behavior.

The Mac must stay awake and Docker Desktop must stay running. The user service
starts at login. On this machine the runner is installed in
`/Users/a.kush/.local/share/new-api-actions-runner`; run `./svc.sh status`,
`./svc.sh stop`, or `./svc.sh start` from that directory to manage it.

Only trusted release workflows should use this runner; do not route untrusted
pull requests to it. Production servers still pull ready registry images by an
explicit tag. Building on this Mac does not deploy or restart production services.
