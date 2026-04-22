package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveSecret reads a secret value from the process environment, with a
// file-backed fallback for systemd-creds-style delivery.
//
// Order:
//
//  1. If `envVar` is set AND non-empty, return that value.
//  2. Else if the process environment has `CREDENTIALS_DIRECTORY` set
//     (systemd 250+ injects this when `LoadCredential=` / `LoadCredentialEncrypted=`
//     is configured on the unit), read `<CREDENTIALS_DIRECTORY>/<credName>`
//     and return its trimmed contents.
//  3. Else return ("", nil) — the caller decides whether empty is fatal.
//
// A read error on the credentials file is returned as-is (wraps %w).
// A missing credentials file (when CREDENTIALS_DIRECTORY is set but the
// named entry isn't there) is NOT an error — it returns ("", nil) so a
// caller that has an env-var fallback path can still decide what to do.
//
// The file is trimmed of surrounding ASCII whitespace so that operators who
// `echo 'value' | systemd-creds encrypt ...` don't end up with a sneaky
// trailing newline in the secret.
func ResolveSecret(envVar, credName string) (string, error) {
	if v := strings.TrimSpace(os.Getenv(envVar)); v != "" {
		return v, nil
	}
	credDir := os.Getenv("CREDENTIALS_DIRECTORY")
	if credDir == "" {
		return "", nil
	}
	path := filepath.Join(credDir, credName)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read credential %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

// SecretSource describes where a secret was loaded from. Returned by
// ResolveSecretWithSource; used by doctor + logging to tell operators which
// delivery mode is in use without echoing the value itself.
type SecretSource int

const (
	SecretSourceNone  SecretSource = iota // not found anywhere
	SecretSourceEnv                       // process environment
	SecretSourceCreds                     // $CREDENTIALS_DIRECTORY file
)

func (s SecretSource) String() string {
	switch s {
	case SecretSourceEnv:
		return "env"
	case SecretSourceCreds:
		return "systemd-creds"
	default:
		return "unset"
	}
}

// ResolveSecretWithSource is ResolveSecret plus a tag describing which
// source the value came from. Callers that need to *display* the mode (e.g.
// doctor / startup log) use this. Callers that just need the value use
// ResolveSecret.
func ResolveSecretWithSource(envVar, credName string) (string, SecretSource, error) {
	if v := strings.TrimSpace(os.Getenv(envVar)); v != "" {
		return v, SecretSourceEnv, nil
	}
	credDir := os.Getenv("CREDENTIALS_DIRECTORY")
	if credDir == "" {
		return "", SecretSourceNone, nil
	}
	path := filepath.Join(credDir, credName)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", SecretSourceNone, nil
		}
		return "", SecretSourceNone, fmt.Errorf("read credential %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), SecretSourceCreds, nil
}
