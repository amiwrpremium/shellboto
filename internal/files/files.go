package files

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const maxDownloadBytes = 50 * 1024 * 1024 // Telegram sendDocument cap via bot API

// Send reads path and uploads it to the chat as a document. Returns the
// byte size that was sent on success.
func Send(ctx context.Context, b *bot.Bot, chatID int64, path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if info.IsDir() {
		return 0, fmt.Errorf("%s is a directory", path)
	}
	if info.Size() > maxDownloadBytes {
		return 0, fmt.Errorf("file is %d bytes, exceeds Telegram 50MB bot limit", info.Size())
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if _, err := b.SendDocument(ctx, &bot.SendDocumentParams{
		ChatID: chatID,
		Document: &models.InputFileUpload{
			Filename: filepath.Base(path),
			Data:     f,
		},
	}); err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// SendBytes uploads an in-memory byte slice as a document.
func SendBytes(ctx context.Context, b *bot.Bot, chatID int64, filename string, data []byte, caption string) error {
	_, err := b.SendDocument(ctx, &bot.SendDocumentParams{
		ChatID: chatID,
		Document: &models.InputFileUpload{
			Filename: filename,
			Data:     bytes.NewReader(data),
		},
		Caption: caption,
	})
	return err
}

// ChownTo optionally re-owns a written file to (Uid,Gid). Nil = skip chown.
type ChownTo struct {
	Uid, Gid uint32
}

// Receive downloads a Telegram document to `<cwd>/<filename>` (or under a
// relative subpath if the message caption gives one). The final destination
// MUST resolve inside cwd — captions with absolute paths or `..` escapes
// are rejected. O_NOFOLLOW is used on the final open to prevent a
// malicious final-component symlink from redirecting the write.
// When `chown` is non-nil, the saved file is chowned to those uid/gid
// (used to make uploads readable/writable for non-root user shells).
func Receive(ctx context.Context, b *bot.Bot, doc *models.Document, caption string, cwd string, chown *ChownTo) (string, int64, error) {
	if doc == nil {
		return "", 0, errors.New("no document")
	}
	dest, err := resolveDest(cwd, caption, sanitizeFilename(doc.FileName))
	if err != nil {
		return "", 0, err
	}
	// resolveDest's containment check is string-level and doesn't
	// catch escapes via INTERMEDIATE symlinks (e.g. cwd/escape → /etc).
	// Re-verify on the filesystem that dest's parent still lives under
	// the real cwd after symlink resolution. Narrow TOCTOU race with an
	// attacker planting a symlink between this check and the MkdirAll /
	// OpenFile below remains — closing it would require openat2 with
	// RESOLVE_BENEATH (Linux 5.6+); accepted for now.
	if err := verifyNoSymlinkEscape(cwd, dest); err != nil {
		return "", 0, err
	}

	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: doc.FileID})
	if err != nil {
		return "", 0, err
	}
	url := b.FileDownloadLink(file)

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download status %s", resp.Status)
	}
	// O_NOFOLLOW: if `dest` already exists and is a symlink, open fails.
	// Prevents the "admin pre-creates a symlink, user uploads to overwrite
	// the target" trick. Intermediate-component symlinks aren't fully
	// guarded here; that's an operator concern (don't symlink from inside
	// user cwds to system paths).
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return "", 0, fmt.Errorf("open dest: %w", err)
	}
	defer out.Close()
	// Defense-in-depth — Telegram's bot API caps uploads at
	// 50 MB, but a misconfigured self-hosted bot API server (or a
	// future protocol change) could serve larger. Bound the Copy with
	// LimitReader(cap+1); if we read the `+1` byte, the body exceeded
	// our cap and we refuse + clean up the partial file rather than
	// quietly filling disk.
	limit := int64(maxDownloadBytes) + 1
	n, err := io.Copy(out, io.LimitReader(resp.Body, limit))
	if err != nil {
		_ = os.Remove(dest)
		return "", 0, err
	}
	if n > int64(maxDownloadBytes) {
		_ = os.Remove(dest)
		return "", 0, fmt.Errorf("upload exceeds %d-byte cap", maxDownloadBytes)
	}
	if chown != nil {
		_ = os.Chown(dest, int(chown.Uid), int(chown.Gid))
	}
	return dest, n, nil
}

// resolveDest computes the final write path for an upload, enforcing the
// "must stay inside cwd" invariant. Returns an error describing why a
// caption was rejected; the caller surfaces that to the user.
func resolveDest(cwd, caption, filename string) (string, error) {
	cleanCwd, err := filepath.Abs(filepath.Clean(cwd))
	if err != nil {
		return "", fmt.Errorf("cwd: %w", err)
	}

	caption = strings.TrimSpace(caption)
	if caption == "" {
		return filepath.Join(cleanCwd, filename), nil
	}

	if filepath.IsAbs(caption) {
		return "", errors.New("absolute caption paths not allowed — cd into the directory first, or use a relative path")
	}

	resolved := filepath.Clean(filepath.Join(cleanCwd, caption))

	// If the caption has a trailing slash OR names an existing directory,
	// place the upload inside it with its original filename. Otherwise
	// treat the caption as the full destination path.
	if strings.HasSuffix(caption, "/") {
		resolved = filepath.Join(resolved, filename)
	} else if st, err := os.Lstat(resolved); err == nil && st.IsDir() {
		resolved = filepath.Join(resolved, filename)
	}

	// Containment: the cleaned destination must be strictly inside cwd.
	rel, err := filepath.Rel(cleanCwd, resolved)
	if err != nil {
		return "", fmt.Errorf("caption path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("caption path %q would escape shell cwd", caption)
	}

	return resolved, nil
}

// verifyNoSymlinkEscape re-asserts that `dest` is contained under `cwd`
// after resolving symlinks on the filesystem. resolveDest only does a
// string-level path-cleanup check, which trusts that no component of
// cwd/... is a symlink pointing elsewhere. This function closes the
// "user pre-planted a symlink in their cwd" escape by walking up from
// dest's parent dir to the deepest existing ancestor, resolving it via
// filepath.EvalSymlinks, and comparing against the resolved cwd.
//
// Non-existent deeper components (e.g. caption creates a new subdir)
// are tolerated: MkdirAll will create them under the already-verified
// real ancestor, and newly-created directories are not symlinks.
func verifyNoSymlinkEscape(cwd, dest string) error {
	realCwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return fmt.Errorf("resolve cwd: %w", err)
	}
	dir := filepath.Dir(dest)
	for {
		if _, err := os.Lstat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return fmt.Errorf("no existing ancestor for %q", dest)
		}
		dir = parent
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("resolve dest parent: %w", err)
	}
	rel, err := filepath.Rel(realCwd, realDir)
	if err != nil {
		return fmt.Errorf("relpath: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return fmt.Errorf("caption path %q escapes shell cwd via symlink", dest)
	}
	return nil
}

func sanitizeFilename(name string) string {
	if name == "" {
		return "upload.bin"
	}
	// strip path separators; Telegram shouldn't send them but be safe.
	name = filepath.Base(name)
	if name == "." || name == ".." || name == "/" {
		return "upload.bin"
	}
	return name
}
