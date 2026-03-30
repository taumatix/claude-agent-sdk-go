## Building go

All go binaries must be built using a plain `go build ${module}`.

Any code generation must be run using `go generate ${module}`.

## Well-known libraries

- *github.com/samber/do* is the preferred dependency injection framework
- *github.com/spf13/cobra* is the preferred command line creation framework
- *https://github.com/danielgtaylor/huma* is the preferred library to expose a REST API
- *github.com/gorilla/mux* is the preferred go HTTP router
