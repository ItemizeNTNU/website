package config

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"strings"
)

// loadDotenv seeds the process environment from a KEY=value file.
//
// A missing file is not an error — .env is a local convenience, and in
// production the environment is injected by the orchestrator. Variables that
// are already set always win, so an explicit `FOO=bar ./website` is never
// silently overridden by a stale file.
//
// Supported: blank lines, `#` comments, `export ` prefixes, values wrapped in
// single or double quotes, and a leading UTF-8 byte-order mark. Escape
// sequences are deliberately not interpreted; a secret containing a backslash
// should survive verbatim.
func loadDotenv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for first := true; sc.Scan(); first = false {
		raw := sc.Text()
		if first {
			// A UTF-8 byte-order mark is what Notepad and friends put at the
			// front of a saved file. It is invisible and it is not whitespace,
			// so without this it becomes part of the first variable's name —
			// and the operator is told FUSION_AUTH_HOST is missing while
			// looking straight at the line that sets it.
			raw = strings.TrimPrefix(raw, "\ufeff")
		}
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, set := os.LookupEnv(key); set {
			continue
		}
		if err := os.Setenv(key, unquote(strings.TrimSpace(value))); err != nil {
			return err
		}
	}
	return sc.Err()
}

// unquote strips one matching pair of surrounding quotes, if present. An
// unquoted value keeps any trailing `#` — treating it as a comment would
// mangle secrets, which routinely contain one.
func unquote(v string) string {
	if len(v) < 2 {
		return v
	}
	if q := v[0]; (q == '"' || q == '\'') && v[len(v)-1] == q {
		return v[1 : len(v)-1]
	}
	return v
}
