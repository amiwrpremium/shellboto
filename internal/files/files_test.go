package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDest(t *testing.T) {
	cwd := t.TempDir()
	// Pre-create an existing directory inside cwd for the "isDir" branch.
	subdir := filepath.Join(cwd, "existing")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cases := []struct {
		name        string
		caption     string
		filename    string
		wantSuffix  string // dest must end with this
		wantErr     bool
		errContains string
	}{
		{
			name:       "no caption → cwd/filename",
			caption:    "",
			filename:   "upload.bin",
			wantSuffix: "upload.bin",
		},
		{
			name:       "simple filename caption",
			caption:    "renamed.txt",
			filename:   "original.bin",
			wantSuffix: "renamed.txt",
		},
		{
			name:       "relative subpath (nonexistent) → full path",
			caption:    "sub/newname.txt",
			filename:   "ignored.bin",
			wantSuffix: "sub/newname.txt",
		},
		{
			name:       "trailing slash → place under dir",
			caption:    "newdir/",
			filename:   "file.bin",
			wantSuffix: "newdir/file.bin",
		},
		{
			name:       "existing directory without trailing slash",
			caption:    "existing",
			filename:   "file.bin",
			wantSuffix: "existing/file.bin",
		},
		{
			name:       "relative .. that resolves back inside cwd",
			caption:    "a/../b.txt",
			filename:   "ignored.bin",
			wantSuffix: "b.txt",
		},

		// Rejections:
		{
			name:        "absolute path rejected",
			caption:     "/etc/sudoers.d/evil",
			filename:    "x.bin",
			wantErr:     true,
			errContains: "absolute caption paths not allowed",
		},
		{
			name:        "absolute root path rejected",
			caption:     "/root/.ssh/authorized_keys",
			filename:    "x.bin",
			wantErr:     true,
			errContains: "absolute caption paths not allowed",
		},
		{
			name:        "dot-dot escape rejected",
			caption:     "../outside.txt",
			filename:    "x.bin",
			wantErr:     true,
			errContains: "escape shell cwd",
		},
		{
			name:        "deep dot-dot escape rejected",
			caption:     "a/b/../../../etc/passwd",
			filename:    "x.bin",
			wantErr:     true,
			errContains: "escape shell cwd",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dest, err := resolveDest(cwd, c.caption, c.filename)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got dest=%q", dest)
				}
				if !strings.Contains(err.Error(), c.errContains) {
					t.Fatalf("err = %q, want contains %q", err, c.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.HasSuffix(dest, c.wantSuffix) {
				t.Fatalf("dest = %q, want suffix %q", dest, c.wantSuffix)
			}
			// dest must be inside cwd.
			if rel, err := filepath.Rel(cwd, dest); err != nil || strings.HasPrefix(rel, "..") {
				t.Fatalf("dest %q escaped cwd %q (rel=%q)", dest, cwd, rel)
			}
		})
	}
}

func TestResolveDestSymlinkInCwdIsContained(t *testing.T) {
	// resolveDest itself is string-based and doesn't follow symlinks —
	// that part is intentional. The secondary filesystem-aware
	// containment check lives in verifyNoSymlinkEscape (tested below).
	cwd := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(cwd, "escape-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	// String-level: resolveDest returns a path inside cwd.
	dest, err := resolveDest(cwd, "escape-link/file.txt", "ignored.bin")
	if err != nil {
		t.Fatalf("resolveDest: %v", err)
	}
	if rel, err := filepath.Rel(cwd, dest); err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("resolveDest escape: dest=%q cwd=%q", dest, cwd)
	}
}

// TestVerifyNoSymlinkEscape_AllowsPlainSubdir: cwd with a plain file or
// a not-yet-existent nested path should pass.
func TestVerifyNoSymlinkEscape_AllowsPlainSubdir(t *testing.T) {
	cwd := t.TempDir()
	// Existing subdir.
	sub := filepath.Join(cwd, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dest := filepath.Join(sub, "file.txt")
	if err := verifyNoSymlinkEscape(cwd, dest); err != nil {
		t.Fatalf("plain subdir rejected: %v", err)
	}
	// Deeper non-existent path: deepest existing ancestor (cwd/sub)
	// is still under cwd → should pass.
	deep := filepath.Join(sub, "new", "deeper", "file.txt")
	if err := verifyNoSymlinkEscape(cwd, deep); err != nil {
		t.Fatalf("not-yet-existent deep path rejected: %v", err)
	}
}

// TestVerifyNoSymlinkEscape_RejectsIntermediateSymlink covers the
// intermediate-symlink escape case — cwd/escape → /etc, caption
// escape/hosts. Must reject.
func TestVerifyNoSymlinkEscape_RejectsIntermediateSymlink(t *testing.T) {
	cwd := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(cwd, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	dest := filepath.Join(link, "payload.txt")
	err := verifyNoSymlinkEscape(cwd, dest)
	if err == nil {
		t.Fatalf("expected escape rejection for dest %q (via symlink %q → %q)",
			dest, link, outside)
	}
	if !strings.Contains(err.Error(), "escape") {
		t.Fatalf("error %q doesn't mention escape", err)
	}
}

// TestVerifyNoSymlinkEscape_ResolvesCwdItself: if cwd is ITSELF a
// symlink to a real dir, we should still accept destinations under
// the real dir. (Admins may set user_shell_home to a symlinked path;
// that shouldn't break uploads.)
func TestVerifyNoSymlinkEscape_ResolvesCwdItself(t *testing.T) {
	realDir := t.TempDir()
	parent := t.TempDir()
	cwdLink := filepath.Join(parent, "cwd-link")
	if err := os.Symlink(realDir, cwdLink); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	dest := filepath.Join(cwdLink, "file.txt")
	if err := verifyNoSymlinkEscape(cwdLink, dest); err != nil {
		t.Fatalf("symlinked cwd should still pass containment: %v", err)
	}
}
