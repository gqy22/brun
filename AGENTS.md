# Agent Notes

## Build and Run

- Use `make build` for local builds. This writes the binary to `bin/brun`.
- Do not run `go build -o brun .` or create a root-level `./brun` binary.
- Use `./bin/brun ...` when testing the freshly built local binary.
- `bin/` is intentionally gitignored as a build output directory.

## Verification

- Run focused tests with `go test ./cmd` or `go test ./internal` when changing those packages.
- Run `go test ./...` before broader commits when changes cross package boundaries.
- For Web UI checks, start the local server with `./bin/brun web --port <port>` after `make build`.
