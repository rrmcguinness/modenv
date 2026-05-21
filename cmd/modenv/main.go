// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/rrmcguinness/modenv/pkg/modenv"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "setup":
		runSetup()
	case "read":
		runRead()
	case "encode", "--encode":
		runEncode()
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Printf("Unknown command: %q\n", command)
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("Usage: modenv <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  setup           Create initial configuration files if they do not exist")
	fmt.Println("  read            Show the resolved configuration tree for the current MODENV_RUNTIME")
	fmt.Println("  encode <value>  Encode a secret value to store safely in config files (prefixed with xor:)")
	fmt.Println("  --encode <val>  Alias for encode")
	fmt.Println("  help            Show this help message")
}

func runSetup() {
	files := map[string]string{
		".env.toml": `app_name = "my-app"
port = 8080
features = ["api"]

[database]
host = "localhost"
user = "root"
password = "plain_db_password"
`,
		".env.local.toml": `# Local environment overrides (never commit this file)
port = 3000

[database]
host = "127.0.0.1"
password = "xor:01000704022949063a1600061f03421901" # Encoded: "local_db_password"
`,
		".env.development.toml": `port = 8000

[database]
host = "dev-db"
`,
		".env.production.toml": `port = 9000

[database]
host = "prod-db-cluster"
`,
	}

	for filename, content := range files {
		destPath := resolvePath(filename)
		if fileExists(destPath) {
			fmt.Printf("Skipping %s (already exists)\n", destPath)
			continue
		}

		dir := filepath.Dir(destPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", dir, err)
			os.Exit(1)
		}

		err := os.WriteFile(destPath, []byte(content), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", destPath, err)
			os.Exit(1)
		}
		fmt.Printf("Created %s\n", destPath)
	}
}

func runRead() {
	baseFile := resolvePath(".env.toml")
	if !fileExists(baseFile) {
		fmt.Fprintf(os.Stderr, "Error: Base configuration file %s is missing. Run 'modenv setup' to initialize configuration files.\n", baseFile)
		os.Exit(1)
	}

	cfg, err := modenv.Load(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	runtime := os.Getenv("MODENV_RUNTIME")
	if runtime == "" {
		runtime = "(none)"
	}
	prefix := os.Getenv("MODENV_PREFIX")
	if prefix == "" {
		prefix = "(working directory)"
	}
	fmt.Printf("# Resolved Configuration Tree (MODENV_RUNTIME=%s, MODENV_PREFIX=%s)\n", runtime, prefix)

	err = toml.NewEncoder(os.Stdout).Encode(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding resolved configuration: %v\n", err)
		os.Exit(1)
	}
}

func runEncode() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Error: Missing secret to encode. Usage: modenv encode <secret-value>\n")
		os.Exit(1)
	}
	secret := os.Args[2]
	encoded := modenv.EncryptSecret(secret)
	fmt.Println(encoded)
}

func resolvePath(filename string) string {
	prefix := os.Getenv("MODENV_PREFIX")
	if prefix == "" {
		return filename
	}
	return filepath.Join(prefix, filename)
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
