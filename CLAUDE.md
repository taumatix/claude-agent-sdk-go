## Architectural principles

Our code is driven by the following founding principles.

- A little copying is better than a little dependency.
  avoid adding a new dependency for a small function that could be written in a few lines
- The bigger the interface, the weaker the abstraction.
  restrict interface definitions to the minimum strictly required methods
- Don't repeat yourself, factorize common features.
  prefer small and readable well-named and re-ueable functions to large ones
- Don't re-invent the wheel you need.
  if a library already exist for the specific use-case you need and prevents large codebase, just import it and use it.

## File architecture

### Self contained domain modules

We prefer vertically sliced modules. A given go module should contain all the business logic for a specific feature.
The different components (HTTP server, service business logic, configuration, database accesses) are split in different files within the module.

Domains are usually stored at the root of the project inside the `domains` folder.

Large domains can be recursively split in sub-domains (i.e. `domains/${domain}/${sub-domain}[/...]`)

### Command line

All executables are stored in `./cmd/command-name`

We follow the convention that all the commands a repository supports are stored in `./cmd/command-name/main.go`.

Commands are usually stored at the root of the project.

Command line programs usually imports commands and sub-commands from `./shared/commands` to maximize re-ueability.

### Shared libraries

Any technical helper should be in `shared/${utility}` and must be exempt from any domain specific features.
Technical helpers must only contains opinionated that can be shared across the project to increase domain code readability.

## Examples

### Architecture of a simple web API

```
cmd/server/main.go # Starts and stops the seb server importing all the relevant domains and helpers
domains/users/http.go # Contains all the HTTP server functions (ServeHTTP) and pure HTTP transport handling
domains/users/rest.go # Contains all REST management (serialization, deserialization, error handling, ...)
domains/users/service.go # Contains the pure business logic for the current user
domains/users/database.go # Contains the connector to store and load typed data from the database
domains/users/config.go # Contains all the configuration for the service, database and HTTP when required
```

### Command line architecture

```
cmd/my-cli/main.go # Contains the main() function and initializes all its sub-commands from `commands`
commands/list.go # Contains the definition of the `my-cli list` command
```
