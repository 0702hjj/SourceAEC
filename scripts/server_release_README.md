# SourceAEC server binaries

This server-only distribution contains the Go gateway (`bin/sourceaec-server`)
and business runtime (`bin/sourceaec-runtime`) for Linux. It does not contain
frontend assets, the converter, Python services, or skill environments.

Use the matching SourceAEC source tag for these additional components and follow
[the deployment guide](https://0702hjj.github.io/SourceAEC/guide/quickstart).
PostgreSQL is required. IFC/CAD script execution also requires Python 3.10, uv,
Git and bubblewrap; conversion requires Node.js 22 and converter dependencies.

1. Extract the archive alongside the matching source checkout, or copy its
   `bin/` contents into the checkout's `bin/` directory.
2. Copy `server/server_config.example.json` to `server/server_config.json` and
   configure paths for your deployment. Default relative paths assume the
   gateway runs with the source checkout's `server/` as its working directory.
3. Set `VIEWER_PG_DSN` for the gateway and both Python services. Use the same
   absolute `VIEWER_DATA_DIR` for all three processes.
4. Set `SOURCEAEC_RUNTIME_BIN` to the absolute path of `bin/sourceaec-runtime`
   for both Python services. Bind them to loopback ports 8100 and 8200.
5. Configure `VIEWER_API_TOKEN` and `VIEWER_CORS_ORIGINS` before exposing the
   gateway. Run `../bin/sourceaec-server` from the checkout's `server/` directory.
   To serve the browser UI, build `web/dist` separately.

Verify the download with `sha256sum -c SHA256SUMS`. Each binary includes an
adjacent build information file with the compiler and module versions.

The optional legacy SQLite migration command is not included. Use the matching
source checkout and a CGO-enabled Go build if migration is needed.
