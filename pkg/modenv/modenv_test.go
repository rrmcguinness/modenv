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

package modenv_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rrmcguinness/modenv/pkg/modenv"
	"github.com/stretchr/testify/assert"
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
	Password string `toml:"password"`
}

func getTestConfigsDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "../../test/configs" // fallback
	}
	for {
		// Check for test/configs (when run from workspace root or pkg/)
		targetTestConfigs := filepath.Join(dir, "test", "configs")
		if info, err := os.Stat(targetTestConfigs); err == nil && info.IsDir() {
			return targetTestConfigs
		}
		// Check for configs (when run from test/)
		targetConfigs := filepath.Join(dir, "configs")
		if info, err := os.Stat(targetConfigs); err == nil && info.IsDir() {
			return targetConfigs
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "../../test/configs"
}

func TestSecretEncryptionDecryptionRoundtrip(t *testing.T) {
	plain := "my-super-secret-password-123!"

	// Test default key
	encoded := modenv.EncryptSecret(plain)
	assert.Contains(t, encoded, "xor:")
	decoded, err := modenv.DecryptSecret(encoded)
	assert.NoError(t, err)
	assert.Equal(t, plain, decoded)

	// Test custom key
	os.Setenv("MODENV_KEY", "custom-super-long-key-spec")
	defer os.Unsetenv("MODENV_KEY")

	encodedCustom := modenv.EncryptSecret(plain)
	assert.NotEqual(t, encoded, encodedCustom)
	decodedCustom, err := modenv.DecryptSecret(encodedCustom)
	assert.NoError(t, err)
	assert.Equal(t, plain, decodedCustom)
}

func TestLoad_WithEncryptedSecrets(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "secrets")
	t.Setenv("MODENV_PREFIX", dir)

	res, err := modenv.Load(nil)
	assert.NoError(t, err)

	cfgMap, ok := res.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "secret-app", cfgMap["app_name"])

	dbMap, ok := cfgMap["database"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "local_db_password", dbMap["password"])
}

func TestLoad_WithPrefix(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "prefix-test")
	t.Setenv("MODENV_PREFIX", dir)

	res, err := modenv.Load(nil)
	assert.NoError(t, err)

	cfgMap, ok := res.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "prefixed-app", cfgMap["app_name"])
	assert.Equal(t, int64(2000), cfgMap["port"])
}

func TestLoad_MapDefault(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "default")
	t.Setenv("MODENV_PREFIX", dir)

	res, err := modenv.Load(nil)
	assert.NoError(t, err)

	cfgMap, ok := res.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "modenv-default", cfgMap["app_name"])
	assert.Equal(t, int64(8080), cfgMap["port"])

	dbMap, ok := cfgMap["database"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "localhost", dbMap["host"])
}

func TestLoad_Struct(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "struct")
	t.Setenv("MODENV_PREFIX", dir)

	var cfg Config
	cloneRes, err := modenv.Load(&cfg)
	assert.NoError(t, err)

	assert.Equal(t, "modenv-struct", cfg.AppName)
	assert.Equal(t, 9000, cfg.Port)
	assert.Equal(t, []string{"web"}, cfg.Features)
	assert.Equal(t, "db", cfg.Database.Host)

	cloneCfg, ok := cloneRes.(*Config)
	assert.True(t, ok)
	assert.Equal(t, "modenv-struct", cloneCfg.AppName)
	assert.Equal(t, 9000, cloneCfg.Port)
}

func TestLoad_RuntimeOverride(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "runtime")
	t.Setenv("MODENV_PREFIX", dir)
	os.Setenv("MODENV_RUNTIME", "production")
	defer os.Unsetenv("MODENV_RUNTIME")

	res, err := modenv.Load(nil)
	assert.NoError(t, err)

	cfgMap := res.(map[string]interface{})
	assert.Equal(t, "base", cfgMap["app_name"])
	assert.Equal(t, int64(9999), cfgMap["port"])
	dbMap := cfgMap["database"].(map[string]interface{})
	assert.Equal(t, "prod-db", dbMap["host"])
}

func TestLoad_LocalOverrideLast(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "local-override")
	t.Setenv("MODENV_PREFIX", dir)
	os.Setenv("MODENV_RUNTIME", "production")
	defer os.Unsetenv("MODENV_RUNTIME")

	res, err := modenv.Load(nil)
	assert.NoError(t, err)

	cfgMap := res.(map[string]interface{})
	assert.Equal(t, "prod", cfgMap["app_name"])
	assert.Equal(t, int64(7777), cfgMap["port"])
	dbMap := cfgMap["database"].(map[string]interface{})
	assert.Equal(t, "local-db", dbMap["host"])
}

func TestLoad_DefensiveCopy(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "defensive")
	t.Setenv("MODENV_PREFIX", dir)

	var cfg Config
	cloneRes, err := modenv.Load(&cfg)
	assert.NoError(t, err)

	cloneCfg, ok := cloneRes.(*Config)
	assert.True(t, ok)

	assert.Equal(t, "defensive", cfg.AppName)
	assert.Equal(t, "defensive", cloneCfg.AppName)

	cloneCfg.AppName = "mutated"
	cloneCfg.Features[0] = "mutated-feature"
	cloneCfg.Database.Host = "mutated-host"

	assert.Equal(t, "defensive", cfg.AppName)
	assert.Equal(t, "original", cfg.Features[0])
	assert.Equal(t, "original-host", cfg.Database.Host)
}

func TestLoad_MissingBaseFileReturnsError(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "missing")
	t.Setenv("MODENV_PREFIX", dir)

	res, err := modenv.Load(nil)
	assert.Error(t, err)
	assert.Nil(t, res)
}

func TestEnvManager(t *testing.T) {
	const preExistingKey = "TEST_PRE_EXISTING"
	const preExistingVal = "original_value"
	err := os.Setenv(preExistingKey, preExistingVal)
	assert.NoError(t, err)
	defer func() {
		_ = os.Unsetenv(preExistingKey)
	}()

	tests := []struct {
		name     string
		action   func(m *modenv.EnvManager)
		verify   func(t *testing.T, m *modenv.EnvManager)
		expected map[string]string
	}{
		{
			name: "Set new variable",
			action: func(m *modenv.EnvManager) {
				err := m.Set("TEST_NEW_KEY", "new_val")
				assert.NoError(t, err)
			},
			verify: func(t *testing.T, m *modenv.EnvManager) {
				val, exists := m.Lookup("TEST_NEW_KEY")
				assert.True(t, exists)
				assert.Equal(t, "new_val", val)
			},
			expected: map[string]string{
				"TEST_NEW_KEY": "",
			},
		},
		{
			name: "Modify existing variable",
			action: func(m *modenv.EnvManager) {
				err := m.Set(preExistingKey, "modified_val")
				assert.NoError(t, err)
			},
			verify: func(t *testing.T, m *modenv.EnvManager) {
				val, exists := m.Lookup(preExistingKey)
				assert.True(t, exists)
				assert.Equal(t, "modified_val", val)
			},
			expected: map[string]string{
				preExistingKey: preExistingVal,
			},
		},
		{
			name: "Unset existing variable",
			action: func(m *modenv.EnvManager) {
				err := m.Unset(preExistingKey)
				assert.NoError(t, err)
			},
			verify: func(t *testing.T, m *modenv.EnvManager) {
				_, exists := m.Lookup(preExistingKey)
				assert.False(t, exists)
			},
			expected: map[string]string{
				preExistingKey: preExistingVal,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := modenv.New()
			tt.action(m)
			tt.verify(t, m)
			err := m.Restore()
			assert.NoError(t, err)

			for key, expectedVal := range tt.expected {
				val, exists := os.LookupEnv(key)
				if expectedVal == "" {
					assert.False(t, exists, "key %s should not exist after restore", key)
				} else {
					assert.True(t, exists, "key %s should exist after restore", key)
					assert.Equal(t, expectedVal, val)
				}
			}
		})
	}
}
