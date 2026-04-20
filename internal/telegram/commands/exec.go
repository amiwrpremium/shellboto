package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"
	"go.uber.org/zap"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
	"github.com/amiwrpremium/shellboto/internal/redact"
	"github.com/amiwrpremium/shellboto/internal/shell"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
	"github.com/amiwrpremium/shellboto/internal/telegram/keyboards"
	ns "github.com/amiwrpremium/shellboto/internal/telegram/namespaces"
)

// ShellOptsFor returns the SpawnOpts appropriate for the caller's role.
//   - admin+ (or UserShellCreds unset): nil creds → root shell (current behavior).
//   - user role with creds configured: runs as the configured Unix user with a
//     per-telegram-user home directory under UserShellHome.
//
// For the user-role path, this function ensures the per-user home dir
// exists and is chown'd correctly.
func ShellOptsFor(d *deps.Deps, u *dbm.User) shell.SpawnOpts {
	if u.IsAdminOrAbove() || d.UserShellCreds == nil {
		return shell.SpawnOpts{}
	}
	home := filepath.Join(d.UserShellHome, strconv.FormatInt(u.TelegramID, 10))
	ensureUserHome(d, home)
	return shell.SpawnOpts{
		Creds: d.UserShellCreds,
		Dir:   home,
		Env: []string{
			"HOME=" + home,
			"USER=" + d.Cfg.UserShellUser,
			"LOGNAME=" + d.Cfg.UserShellUser,
			"SHELL=/bin/bash",
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		},
	}
}

// isUsableDir reports whether `path` is either a real directory we can
// safely operate on, OR doesn't exist yet (so MkdirAll will create it
// cleanly). Refuses symlinks — a symlink at this path would redirect
// privileged os.Chown to an attacker-chosen target like /etc, which is
// exactly the escalation path this function guards against. Refuses
// non-directory files too (wrong type, not our path).
func isUsableDir(path string) (ok bool, reason string) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, "" // MkdirAll will create safely
		}
		return false, fmt.Sprintf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, "path is a symlink"
	}
	if !info.IsDir() {
		return false, "path exists but is not a directory"
	}
	return true, ""
}

// ensureUserHome creates (if missing) the per-telegram-user home dir
// and chowns it to the configured shell user so bash-as-that-user can
// write. Failures are logged, not surfaced — if the home can't be set
// up, the spawn will still try (likely landing in /) which bash handles.
//
// Refuses to operate on paths that are symlinks or non-directory
// files. A shellboto-user with write access to the parent directory
// could otherwise pre-plant `/home/shellboto-user/<tid> → /etc`, and
// our subsequent Chown (which follows symlinks) would re-own /etc to
// shellboto-user → full privilege escalation. The parent dir should be
// root-owned (see README) so planting is impossible in the first place;
// this check is defense-in-depth.
func ensureUserHome(d *deps.Deps, dir string) {
	if ok, reason := isUsableDir(dir); !ok {
		d.L().Warn("ensureUserHome refusing unsafe path",
			zap.String("dir", dir),
			zap.String("reason", reason))
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		d.L().Warn("ensureUserHome mkdir", zap.String("dir", dir), zap.Error(err))
		return
	}
	if d.UserShellCreds != nil {
		// Lchown (not Chown): doesn't follow symlinks. Belt-and-
		// suspenders with isUsableDir above — a TOCTOU race that
		// replaces `dir` with a symlink between the Lstat check and
		// here would cause Chown to follow, but Lchown modifies only
		// the symlink inode itself. /etc stays untouched either way.
		if err := os.Lchown(dir, int(d.UserShellCreds.Uid), int(d.UserShellCreds.Gid)); err != nil {
			d.L().Warn("ensureUserHome chown", zap.String("dir", dir), zap.Error(err))
		}
	}
}

// escalationRegex catches any use of a privilege-elevation command as a
// word token: sudo, su, pkexec, doas, runuser, setpriv. Non-admins
// matching this (or any danger pattern) are banned; admins are
// unaffected (they already have root).
//
// Not airtight — bash syntax tricks ($'\163”udo', "s""udo", x=sudo;$x,
// base64 → sh) defeat any regex. The real defense against privilege
// elevation by user-role callers is OS-level isolation (non-root shells
// via user_shell_user).
var escalationRegex = regexp.MustCompile(`\b(sudo|su|pkexec|doas|runuser|setpriv)\b`)

// isPrintableASCII returns true iff every byte in s is printable ASCII
// (0x20–0x7E) or one of tab / LF / CR. Null, DEL, and any byte ≥ 0x80
// (start of a UTF-8 multibyte sequence) fail the check.
func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		if c < 0x20 || c > 0x7E {
			return false
		}
	}
	return true
}

// RequireAdmin enforces admin+ on text commands. Replies with a polite
// rejection on failure.
func RequireAdmin(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) bool {
	if u.IsAdminOrAbove() {
		return true
	}
	common.Reply(ctx, b, update.Message.Chat.ID, "admin or superadmin only")
	return false
}

// ExecShell is the shared entry point for running a user-typed command
// against that user's pty shell. Called from the default text handler.
func ExecShell(ctx context.Context, d *deps.Deps, b *bot.Bot, chatID int64, u *dbm.User, cmd string) {
	// Optional strict-ASCII filter (opt-in). Rejects non-ASCII or
	// non-printable bytes before any regex check. Does NOT ban —
	// mispastes shouldn't punish.
	if d.Cfg.StrictASCIICommands && !isPrintableASCII(cmd) {
		common.Reply(ctx, b, chatID,
			"command rejected: contains non-ASCII or non-printable bytes (strict_ascii_commands is enabled)")
		uid := u.TelegramID
		_, _ = d.Audit.Log(ctx, repo.Event{
			UserID: &uid, Kind: dbm.KindAuthReject,
			Cmd:    cmd,
			Detail: map[string]any{"reason": "strict_ascii_commands"},
		})
		return
	}

	hasEsc := escalationRegex.MatchString(cmd)
	pat, hasDanger := d.Danger.Match(cmd)

	// Non-admin attempting privilege escalation / dangerous command → ban.
	if !u.IsAdminOrAbove() && (hasEsc || hasDanger) {
		reason := "dangerous pattern: " + pat
		if hasEsc {
			reason = "privilege-escalation attempt: " + escalationRegex.FindString(cmd)
		}
		BanUser(ctx, d, b, chatID, u, cmd, reason)
		return
	}

	if hasDanger {
		tok := d.Confirm.Stash(u.TelegramID, cmd)
		uid := u.TelegramID
		// Log redacted cmd — this is a rare, high-signal event so
		// operators want forensic detail; redact scrubs obvious secrets.
		d.L().Warn("danger match",
			zap.Int64("user_id", uid),
			zap.String("pattern", pat),
			zap.String("cmd_redacted", redact.RedactString(cmd)))
		_, _ = d.Audit.Log(ctx, repo.Event{
			UserID: &uid, Kind: dbm.KindDangerRequested,
			Cmd: cmd, DangerPattern: pat,
		})
		// cmd is echoed back to the caller's chat, where it sits
		// for at least `confirm_ttl` (and in practice forever). Redact
		// before truncating so secrets in the flagged command don't
		// leak into chat history — the audit row already hashes the
		// redacted form, so both views stay consistent.
		text := fmt.Sprintf(
			"⚠ dangerous pattern matched: %s\ncmd: %s\ntap below within %s to run",
			pat, common.Truncate(redact.RedactString(cmd), 200), d.Cfg.ConfirmTTL.Duration.Round(time.Second))
		kb := keyboards.YesNoTokenized(ns.Danger, ns.Yes, ns.No, tok)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID, Text: text, ReplyMarkup: kb,
		})
		return
	}
	DispatchShell(ctx, d, b, chatID, u, cmd)
}

// DispatchShell runs an already-vetted command and streams its output.
// Separated from ExecShell so the danger-confirm callback can reuse it.
//
// The audit row is written from a deferred closure at the top of
// the function so a panic inside Streamer.Stream (e.g., a
// malformed-response nil-deref in the Telegram library) can't skip
// the audit. The command already ran on the shell by the time Stream
// starts, so dropping the audit row would lose forensic record of
// work that actually happened. An inner deferred recover captures
// panic state into a flag the audit picks up, then re-panics so
// middleware.recoverHandler still catches it upstream.
func DispatchShell(ctx context.Context, d *deps.Deps, b *bot.Bot, chatID int64, u *dbm.User, cmd string) {
	uid := u.TelegramID
	s, err := d.Shells.Get(uid, ShellOptsFor(d, u))
	if err != nil {
		d.L().Error("shell.Get", zap.Int64("user_id", uid), zap.Error(err))
		common.Reply(ctx, b, chatID, "failed to open shell: "+err.Error())
		return
	}
	j, err := s.Run(cmd)
	if err != nil {
		if err == shell.ErrBusy {
			common.Reply(ctx, b, chatID, "a command is already running — /status, /cancel, /kill")
			return
		}
		d.L().Error("shell.Run", zap.Int64("user_id", uid), zap.Error(err))
		common.Reply(ctx, b, chatID, "run failed: "+err.Error())
		return
	}

	// Deferred audit (runs on normal return AND on panic unwind).
	// Declared first → runs LAST in the LIFO order, after the inner
	// recover flag has settled.
	var streamPanicked bool
	defer func() {
		snap, _ := j.Snapshot()
		exit := j.ExitCode()
		bytesOut := len(snap)
		durMS := j.Duration().Milliseconds()
		termination := j.Termination()
		if exit == -1 {
			termination = "shell_died"
		}
		var detail map[string]any
		if j.Truncated() {
			detail = map[string]any{"output_truncated": true, "cap_bytes": d.Cfg.MaxOutputBytes}
		}
		if streamPanicked {
			if detail == nil {
				detail = map[string]any{}
			}
			detail["stream_panicked"] = true
			if termination == "" || termination == "completed" {
				termination = "stream_panicked"
			}
		}
		if _, err := d.Audit.Log(ctx, repo.Event{
			UserID:      &uid,
			Kind:        dbm.KindCommandRun,
			Cmd:         cmd,
			ExitCode:    &exit,
			BytesOut:    &bytesOut,
			DurationMS:  &durMS,
			Termination: termination,
			Detail:      detail,
			OutputBody:  snap,
		}); err != nil {
			d.L().Warn("audit command_run", zap.Error(err))
		}
	}()

	// Inner recover: catches a panic from Stream, flags it for the
	// audit defer above, and re-panics so the middleware-level
	// recoverHandler still sees + logs it.
	defer func() {
		if r := recover(); r != nil {
			streamPanicked = true
			panic(r)
		}
	}()

	// Every-command Info log: drop raw cmd text entirely. The full
	// redacted cmd is persisted in the audit row + audit journal,
	// so nothing is lost forensically — this just keeps the
	// noisy path in journald secret-free.
	d.L().Info("run", zap.Int64("user_id", uid), zap.Int("cmd_len", len(cmd)))

	timeout := d.Cfg.DefaultTimeout.Duration
	grace := d.Cfg.KillGrace.Duration
	go func() {
		// time.NewTimer + defer Stop so the timers are cleaned
		// up promptly when the command finishes early. time.After
		// leaked a live timer per command for up to `default_timeout`
		// (5m) — bounded but sloppy under moderate load.
		tTimeout := time.NewTimer(timeout)
		defer tTimeout.Stop()
		select {
		case <-j.Done:
			return
		case <-tTimeout.C:
		}
		j.SetTermination("timeout")
		_ = s.SigInt()
		tGrace := time.NewTimer(grace)
		defer tGrace.Stop()
		select {
		case <-j.Done:
			return
		case <-tGrace.C:
		}
		_ = s.SigKill()
	}()

	d.Streamer.Stream(ctx, chatID, j)
}

// BanUser tells the user they're banned, soft-deletes them, kills their
// shell, and audits the ban.
func BanUser(ctx context.Context, d *deps.Deps, b *bot.Bot, chatID int64, u *dbm.User, cmd, reason string) {
	uid := u.TelegramID
	common.Reply(ctx, b, chatID, fmt.Sprintf("🚫 you have been banned: %s", reason))

	if err := d.Users.SoftDelete(uid); err != nil {
		d.L().Error("ban SoftDelete", zap.Int64("user_id", uid), zap.Error(err))
	}
	d.Shells.Reset(uid)

	// Bans are rare + forensically important. Log redacted cmd so
	// admins can understand what the banned user tried to run.
	d.L().Warn("user banned",
		zap.Int64("user_id", uid),
		zap.String("reason", reason),
		zap.String("cmd_redacted", common.Truncate(redact.RedactString(cmd), 200)),
	)
	_, _ = d.Audit.Log(ctx, repo.Event{
		UserID: &uid, Kind: dbm.KindUserBanned,
		Cmd:    cmd,
		Detail: map[string]any{"reason": reason, "was_role": u.Role},
	})
	if d.Notify != nil {
		d.Notify.Banned(ctx, b, u, reason)
	}
}

// WithUploadIndicator keeps the "sending a file" chat action alive while
// fn runs. Used by /get and /audit-out so users see a status during
// multi-MB transfers.
func WithUploadIndicator(ctx context.Context, d *deps.Deps, chatID int64, fn func() error) error {
	ictx, cancel := context.WithCancel(ctx)
	defer cancel()
	if d.Streamer != nil {
		go d.Streamer.ActionLoop(ictx, chatID, tgm.ChatActionUploadDocument)
	}
	return fn()
}

// ShellCwd returns the caller's bash cwd by reading /proc/<pid>/cwd.
// Returns an error when the link can't be read — typically because the
// shell exited between the caller's Shells.Get and this call, or the
// pid was recycled.
//
// Callers MUST surface this error to the user rather than falling back
// to a default path. The previous fallback to "/root" silently wrote
// user-role uploads into /root, where they were owned by shellboto-user
// but unreadable to that user (because /root is 0700 root) — the upload
// succeeded but the file was stranded.
func ShellCwd(pid int) (string, error) {
	link := fmt.Sprintf("/proc/%d/cwd", pid)
	cwd, err := readlink(link)
	if err != nil {
		return "", fmt.Errorf("read cwd for pid %d: %w", pid, err)
	}
	return cwd, nil
}

// TrimCmdArg strips the leading command word (e.g. "/get") off the text.
func TrimCmdArg(text, cmd string) string {
	return strings.TrimSpace(strings.TrimPrefix(text, cmd))
}
