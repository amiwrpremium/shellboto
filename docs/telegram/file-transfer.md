# File transfer

Two directions: `/get <path>` downloads a file from the VPS to the
user's Telegram chat, and a file attachment uploads a file from
Telegram to the VPS.

Source: [`internal/files/files.go`](../../internal/files/files.go).

## Upload (Telegram → VPS)

### How a user sends a file

Tap the paperclip → **File** (not Photo; Telegram re-encodes
photos). Optional caption: destination path on the VPS.

- **No caption** → file lands in the user's shell cwd as
  `<original-filename>`.
- **Caption is an absolute path** → file lands there.
  `/tmp/image.png` writes to `/tmp/image.png` (creating parent
  dirs if they don't exist, with mode 0700).
- **Caption is a relative path** → relative to the user's shell
  cwd.

### Path hardening

The upload handler runs these checks before writing:

1. **No `..` segments.** Rejects `../etc/passwd` regardless of
   role.
2. **Absolute paths for `role=user`:** rejected unless the path
   starts inside `user_shell_home/<telegram_id>/`. Role=admin+
   has no absolute-path restriction (root shell ⇒ full tree
   access anyway).
3. **Target cannot be a symlink.** `os.Lstat` check; refuses to
   overwrite through a symlink to prevent plant-then-upload
   attacks.

### File perms

The file is written as:

- owner: the shell user (root for admin+, `user_shell_user` for
  role=user).
- mode: `0600`.

### Size limits

- Telegram's Bot API hard cap: 50 MB. Larger uploads are rejected
  by Telegram itself; the bot just sees a failed getFile call.
- No separate shellboto-side cap.

### Audit

`kind=file_upload`. Detail includes destination path + byte count.

## Download (VPS → Telegram)

### Usage

```
/get /var/log/nginx/access.log
```

Reads the file, uploads via `sendDocument`. Message includes a
caption with the path + size.

### Path checks (read-side)

Same hardening as uploads:

- No `..`.
- `role=user` limited to paths inside
  `user_shell_home/<telegram_id>/`.
- Reads are not via symlink — refuses to follow.

### Size limits

Bot API cap: 50 MB per document. Files larger than 50 MB fail
with a clear error reply. shellboto does not attempt multi-part
upload.

For logs larger than 50 MB:

```
tail -c 50000000 /var/log/nginx/access.log > /tmp/slice.log
/get /tmp/slice.log
```

Or:

```
gzip -c /var/log/nginx/access.log > /tmp/access.log.gz
/get /tmp/access.log.gz
```

(Both run from the caller's shell; `/get` happens afterwards.)

### Binary files

Arbitrary bytes. Telegram uploads as `application/octet-stream`;
the user downloads and keeps it. Telegram may preview known
formats (images, PDFs) in-chat; unknown formats download-only.

### Audit

`kind=file_download`. Detail includes source path + byte count.

## Why `/get` and not a shell redirect

You could `cat file` in the shell and get it streamed into
Telegram messages. That works but:

- Binary files get HTML-escaped and mangled.
- Telegram caps each message at 4096 chars.
- No convenient re-download on the user's side.

`/get` sends the raw bytes as a file attachment — binary-safe,
downloadable, obvious name.

## Why uploads via caption, not `/upload`

Telegram's Bot API delivers file attachments with an optional
caption field. We repurpose that field as "destination path" so
uploading is a one-step gesture (paperclip → File → caption path →
Send) instead of a multi-step wizard.

If you forget the caption, the file lands in the shell cwd with
its original name. You can always `mv` it afterwards.

## Not supported

- **Streaming downloads.** File must fit entirely in memory
  during the upload to Bot API. For very large files, use
  `scp` / `rsync` from outside the bot.
- **Recursive directory transfer.** `/get /some/dir` fails (it's
  not a regular file). Tar it up first:
  ```
  tar czf /tmp/dir.tar.gz -C /some/parent dir
  /get /tmp/dir.tar.gz
  ```
- **Pre-signed URL / time-limited links.** Telegram hosts the file
  at a URL for ~1 hour after upload; shellboto doesn't track or
  manage that link.

## Security notes

- **Uploads land with 0600 perms** owned by the shell user. That
  means an admin uploading a file puts a root-owned 0600 file on
  disk. Deliberate — minimises exposure window and forces the
  admin to `chmod` deliberately if it's meant to be world-
  readable.
- **`/get` on `/etc/shadow` for admin** — works. The root shell
  has read access; the bot doesn't add extra gates. If this is
  wrong for your ops posture, run role=user shells and use
  uploads from a user who can't read shadow anyway.

## Read next

- [../shell/user-shells.md](../shell/user-shells.md) — per-user
  home-dir setup (relevant for `role=user` upload path
  resolution).
- [../security/secret-redaction.md](../security/secret-redaction.md)
  — uploads are NOT redacted (unlike shell output). Don't upload
  a raw `.env` into the bot unless you mean to.
