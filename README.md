# modenv

![Coverage](coverage.svg)

A hierarchical TOML configuration and environment loader for Go. `modenv` simplifies configuration parsing by merging cascading configuration files, dynamically resolving environment overrides, and automatically decrypting stored secrets on the fly.

## Authors
- **Ryan McGuinness** (Lead Engineer)
- **Hanna** (AI Pair Programmer)

---

## Features
- **Hierarchical Cascading Configuration**: Merges multiple TOML configurations dynamically based on runtime environments.
- **Precedence Order**:
  1. `.env.toml` (Base configuration, required)
  2. `.env.${MODENV_RUNTIME}.toml` (Optional runtime-specific overrides, e.g., `production`)
  3. `.env.local.toml` (Optional local-only configurations, loaded last to ensure absolute precedence)
- **Prefix Path Routing**: Resolves and reads/writes all configuration files relative to the directory path defined in the `MODENV_PREFIX` environment variable. Defaults to the current working directory if unset.
- **Transparent Struct and Map Binding**: Decodes merged data directly into maps or target Go structs via standard `toml` struct tags.
- **Defensive Copying**: Automatically returns deep, isolated clones of the parsed configurations to prevent pointer mutation leaks.
- **Secure Local Storage**: In-place recursive decryption of encrypted strings marked with the `xor:` prefix.
- **Integrated CLI**: A helper binary containing utility functions for templates setup, configuration reading, and secret encoding.

---

## Installation

Add `modenv` to your Go module dependencies:
```bash
go get github.com/rrmcguinness/modenv
```

---

## Library Utilization

### 1. Basic Map Loading
If you pass `nil` to `modenv.Load()`, it parses configuration files into a `map[string]interface{}`:
```go
package main

import (
	"fmt"
	"log"

	"github.com/rrmcguinness/modenv/pkg/modenv"
)

func main() {
	cfgMap, err := modenv.Load(nil)
	if err != nil {
		log.Fatalf("Failed to load environment: %v", err)
	}

	fmt.Printf("App Name: %v\n", cfgMap["app_name"])
}
```

### 2. Loading into Custom Structs (with Defensive Copy)
You can bind TOML key values directly to a target struct. The loader populates your struct and returns a deep, clean copy of the final config:
```go
package main

import (
	"fmt"
	"log"

	"github.com/rrmcguinness/modenv/pkg/modenv"
)

type Config struct {
	AppName  string   `toml:"app_name"`
	Port     int      `toml:"port"`
	Features []string `toml:"features"`
	Database DBConfig `toml:"database"`
}

type DBConfig struct {
	Host     string `toml:"host"`
	User     string `toml:"user"`
	Password string `toml:"password"` // Encrypted values (prefixed with xor:) decrypt transparently
}

func main() {
	var cfg Config
	
	// Load parses and returns a defensive copy interface
	cloneInterface, err := modenv.Load(&cfg)
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Cast the returned interface to your Config pointer
	appConfig := cloneInterface.(*Config)
	fmt.Printf("Resolved Port: %d\n", appConfig.Port)
	fmt.Printf("Decrypted Password: %s\n", appConfig.Database.Password)
}
```

---

## Secret Encoding

To prevent writing secrets in plain text in config files (like `.env.local.toml`), you can encrypt them using the built-in XOR cipher.

### Key Resolution Order:
1. Environment Variable `MODENV_KEY`
2. Default Fallback Key (`modenv-default-key`)

Values in your TOML config starting with the `xor:` prefix will be decrypted on load in-place:
```toml
[database]
password = "xor:01000704022949063a1600061f03421901" # Decrypted transparently to "local_db_password"
```

---

## CLI Reference

`modenv` comes with a CLI tool to automate management of environment configurations.

### Build the CLI
```bash
make build
```

### Generate Configurations
Scaffold default templates (`.env.toml`, `.env.local.toml`, `.env.development.toml`, `.env.production.toml`) if they do not exist (automatically creates directories if `MODENV_PREFIX` is set):
```bash
# In working directory
./bin/modenv setup

# Under prefix directory
MODENV_PREFIX=config/my-app ./bin/modenv setup
```

### View Resolved Configuration Tree
Read, merge, decrypt, and print the resulting configuration tree in TOML format:
```bash
# Default runtime and working directory
./bin/modenv read

# Target specific environment and prefix directory
MODENV_RUNTIME=production MODENV_PREFIX=config/my-app ./bin/modenv read
```

### Encode a Secret Value
Encrypt sensitive properties for safe TOML inclusion:
```bash
# Encrypt using the default key
./bin/modenv encode "my_db_password"

# Encrypt using a custom environment key
MODENV_KEY=my-prod-key ./bin/modenv encode "my_db_password"
```

---

## Bazel Integration

To import `modenv` in a Bazel-based project using Bazelmod, add the following to your `MODULE.bazel`:

```starlark
bazel_dep(name = "modenv", version = "0.0.1")
git_override(
    module_name = "modenv",
    remote = "https://github.com/rrmcguinness/modenv.git",
    commit = "<commit-hash>", # replace with target commit or tag
)
```

You can then reference the library as a dependency in your `BUILD` files:
```starlark
go_library(
    name = "my_lib",
    srcs = ["main.go"],
    deps = ["@modenv//pkg/modenv"],
)
```

### Build, Run & Test with Bazel
Execute builds, runs, and tests directly within the workspace:
```bash
# Build the binary
bazel build //cmd/modenv

# Run the binary (pass CLI arguments after --)
bazel run //cmd/modenv -- --help

# Run all test suites
bazel test //...
```

---

## Development

Execute tests:
```bash
make test
```

