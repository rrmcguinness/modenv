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

package modenv

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// EnvManager tracks changes to the environment variables and allows reverting them.
type EnvManager struct {
	mu       sync.Mutex
	original map[string]*string // maps key to original value. nil value means it did not exist.
}

// New creates a new EnvManager ready to track environment changes.
func New() *EnvManager {
	return &EnvManager{
		original: make(map[string]*string),
	}
}

// Set sets an environment variable and records its original value if not already tracked.
func (m *EnvManager) Set(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, tracked := m.original[key]; !tracked {
		if val, exists := os.LookupEnv(key); exists {
			m.original[key] = &val
		} else {
			m.original[key] = nil
		}
	}

	return os.Setenv(key, value)
}

// Get retrieves the value of the environment variable named by the key.
func (m *EnvManager) Get(key string) string {
	return os.Getenv(key)
}

// Lookup retrieves the value of the environment variable named by the key and reports if it was present.
func (m *EnvManager) Lookup(key string) (string, bool) {
	return os.LookupEnv(key)
}

// Unset unsets an environment variable and records its original value if not already tracked.
func (m *EnvManager) Unset(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, tracked := m.original[key]; !tracked {
		if val, exists := os.LookupEnv(key); exists {
			m.original[key] = &val
		} else {
			m.original[key] = nil
		}
	}

	return os.Unsetenv(key)
}

// Restore reverts all changes made via this EnvManager to their original values.
func (m *EnvManager) Restore() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for key, valPtr := range m.original {
		var err error
		if valPtr == nil {
			err = os.Unsetenv(key)
		} else {
			err = os.Setenv(key, *valPtr)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Reset tracking map
	m.original = make(map[string]*string)
	return firstErr
}

// Load reads and parses hierarchical TOML environment configurations:
// 1. .env.toml (required)
// 2. .env.${MODENV_RUNTIME}.toml (optional runtime override)
// 3. .env.local.toml (optional local override, loaded last)
//
// Paths are resolved relative to MODENV_PREFIX if it is set.
// If target is nil, it returns a map[string]interface{}.
// If target is a non-nil pointer to a struct or map, it decodes the configuration into target
// and returns a clean, defensive copy of the decoded object.
func Load(target interface{}) (interface{}, error) {
	// 1. Load .env.toml (required)
	baseFile := resolvePath(".env.toml")
	merged, err := loadFile(baseFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load base config %s: %w", baseFile, err)
	}

	// 2. Load .env.${MODENV_RUNTIME}.toml (optional runtime overrides)
	if runtime := os.Getenv("MODENV_RUNTIME"); runtime != "" {
		runtimeFile := fmt.Sprintf(".env.%s.toml", runtime)
		runtimePath := resolvePath(runtimeFile)
		if fileExists(runtimePath) {
			runtimeMap, err := loadFile(runtimePath)
			if err != nil {
				return nil, fmt.Errorf("failed to load runtime config %s: %w", runtimePath, err)
			}
			merged = deepMerge(merged, runtimeMap)
		}
	}

	// 3. Load .env.local.toml (optional local overrides, loaded last)
	localPath := resolvePath(".env.local.toml")
	if fileExists(localPath) {
		localMap, err := loadFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load local config %s: %w", localPath, err)
		}
		merged = deepMerge(merged, localMap)
	}

	// Decrypt configuration secrets in place
	if err := decryptConfigMap(merged); err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets in merged configuration: %w", err)
	}

	// 4. Encode the merged map to TOML format
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(merged); err != nil {
		return nil, fmt.Errorf("failed to encode merged configuration: %w", err)
	}
	tomlStr := buf.String()

	// 5. Decode into target or map and return defensive copy
	if target != nil {
		// Populate user target
		_, err := toml.Decode(tomlStr, target)
		if err != nil {
			return nil, fmt.Errorf("failed to decode merged config into target: %w", err)
		}
		// Return defensive copy
		return clone(target)
	}

	// target is nil, decode into a fresh map and return it
	var res map[string]interface{}
	_, err = toml.Decode(tomlStr, &res)
	if err != nil {
		return nil, fmt.Errorf("failed to decode merged config into result map: %w", err)
	}
	return res, nil
}

// EncryptSecret encrypts a plain text string using XOR and returns a hex encoded string prefixed with "xor:".
func EncryptSecret(plainText string) string {
	key := getSecretKey()
	input := []byte(plainText)
	output := make([]byte, len(input))
	for i := 0; i < len(input); i++ {
		output[i] = input[i] ^ key[i%len(key)]
	}
	return "xor:" + hex.EncodeToString(output)
}

// DecryptSecret decrypts an encoded string starting with "xor:" and returns the plain text.
func DecryptSecret(encodedText string) (string, error) {
	if !strings.HasPrefix(encodedText, "xor:") {
		return "", fmt.Errorf("invalid secret format: missing 'xor:' prefix")
	}
	hexStr := encodedText[4:]
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return "", fmt.Errorf("failed to hex decode secret: %w", err)
	}
	key := getSecretKey()
	output := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		output[i] = data[i] ^ key[i%len(key)]
	}
	return string(output), nil
}

// getSecretKey resolves the encryption key from environment variables with fallback.
func getSecretKey() []byte {
	key := os.Getenv("MODENV_KEY")
	if key == "" {
		key = "modenv-default-key"
	}
	return []byte(key)
}

// decryptConfigMap recursively searches a map for string values starting with "xor:" and decrypts them in-place.
func decryptConfigMap(m map[string]interface{}) error {
	for k, v := range m {
		switch val := v.(type) {
		case string:
			if strings.HasPrefix(val, "xor:") {
				decrypted, err := DecryptSecret(val)
				if err != nil {
					return err
				}
				m[k] = decrypted
			}
		case map[string]interface{}:
			err := decryptConfigMap(val)
			if err != nil {
				return err
			}
		case []interface{}:
			err := decryptConfigSlice(val)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// decryptConfigSlice recursively searches a slice for string values starting with "xor:" and decrypts them in-place.
func decryptConfigSlice(s []interface{}) error {
	for i, v := range s {
		switch val := v.(type) {
		case string:
			if strings.HasPrefix(val, "xor:") {
				decrypted, err := DecryptSecret(val)
				if err != nil {
					return err
				}
				s[i] = decrypted
			}
		case map[string]interface{}:
			err := decryptConfigMap(val)
			if err != nil {
				return err
			}
		case []interface{}:
			err := decryptConfigSlice(val)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// resolvePath returns the target path joined with MODENV_PREFIX if configured.
func resolvePath(filename string) string {
	prefix := os.Getenv("MODENV_PREFIX")
	if prefix == "" {
		return filename
	}
	return filepath.Join(prefix, filename)
}

// loadFile reads a file and decodes it into a map.
func loadFile(filename string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	_, err = toml.Decode(string(data), &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// fileExists checks if a file exists and is not a directory.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// deepMerge recursively merges src map into dst map.
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		if dstVal, exists := dst[k]; exists {
			dstMap, dstIsMap := dstVal.(map[string]interface{})
			srcMap, srcIsMap := v.(map[string]interface{})
			if dstIsMap && srcIsMap {
				dst[k] = deepMerge(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
	return dst
}

// clone creates a clean, defensive copy of the source object.
func clone(src interface{}) (interface{}, error) {
	if src == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(src); err != nil {
		return nil, err
	}

	val := reflect.ValueOf(src)
	if val.Kind() != reflect.Ptr {
		dstPtr := reflect.New(val.Type())
		_, err := toml.Decode(buf.String(), dstPtr.Interface())
		if err != nil {
			return nil, err
		}
		return dstPtr.Elem().Interface(), nil
	}

	dstPtr := reflect.New(val.Elem().Type())
	_, err := toml.Decode(buf.String(), dstPtr.Interface())
	if err != nil {
		return nil, err
	}
	return dstPtr.Interface(), nil
}
