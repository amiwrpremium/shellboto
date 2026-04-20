package middleware

import (
	"context"
	"path/filepath"
	"testing"

	tgm "github.com/go-telegram/bot/models"
	"go.uber.org/zap"

	"github.com/amiwrpremium/shellboto/internal/db"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
	"github.com/amiwrpremium/shellboto/internal/telegram/ratelimit"
)

// TestAuditReject_PreAuthLimiterGates locks in the pre-auth audit-reject
// rate limiter. With AuthRejectLimit set to burst=2, the first two
// reject calls from the same From-id write audit rows; the third must
// be silently dropped (no audit row).
func TestAuditReject_PreAuthLimiterGates(t *testing.T) {
	dir := t.TempDir()
	gormDB, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(gormDB) })
	auditRepo := repo.NewAuditRepo(gormDB, nil, nil, repo.OutputAlways, 0)
	d := &deps.Deps{
		Audit:           auditRepo,
		Log:             zap.NewNop(),
		AuthRejectLimit: ratelimit.New(2, 0), // burst=2, no refill
	}
	ctx := context.Background()

	// Three rejects from the same user → only first 2 write rows.
	auditReject(ctx, d, 42, "alice", "Alice", "text", nil)
	auditReject(ctx, d, 42, "alice", "Alice", "text", nil)
	auditReject(ctx, d, 42, "alice", "Alice", "text", nil) // over-limit → drop

	rows, err := auditRepo.Recent(ctx, nil, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 audit rows (burst=2), got %d", len(rows))
	}

	// Different user — gets their own bucket, so first 2 succeed.
	auditReject(ctx, d, 99, "bob", "Bob", "text", nil)
	rows, _ = auditRepo.Recent(ctx, nil, 10)
	if len(rows) != 3 {
		t.Fatalf("separate user should get fresh bucket, got total rows = %d", len(rows))
	}
}

// TestAuditReject_UnlimitedWhenLimiterDisabled — with burst=0, the
// limiter is effectively off, every call writes a row.
func TestAuditReject_UnlimitedWhenLimiterDisabled(t *testing.T) {
	dir := t.TempDir()
	gormDB, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(gormDB) })
	auditRepo := repo.NewAuditRepo(gormDB, nil, nil, repo.OutputAlways, 0)
	d := &deps.Deps{
		Audit:           auditRepo,
		Log:             zap.NewNop(),
		AuthRejectLimit: ratelimit.New(0, 0), // burst=0 → disabled
	}
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		auditReject(ctx, d, 42, "alice", "Alice", "text", nil)
	}
	rows, _ := auditRepo.Recent(ctx, nil, 10)
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows with limiter disabled, got %d", len(rows))
	}
}

func TestTrimUpdate_MessageTextAndCaption(t *testing.T) {
	u := &tgm.Update{
		Message: &tgm.Message{
			Text:    "  hello world  \n",
			Caption: "\t  payload.txt \r\n",
		},
	}
	TrimUpdate(u)
	if u.Message.Text != "hello world" {
		t.Errorf("text = %q, want %q", u.Message.Text, "hello world")
	}
	if u.Message.Caption != "payload.txt" {
		t.Errorf("caption = %q, want %q", u.Message.Caption, "payload.txt")
	}
}

func TestTrimUpdate_CallbackData(t *testing.T) {
	u := &tgm.Update{CallbackQuery: &tgm.CallbackQuery{Data: "  us:p:42  "}}
	TrimUpdate(u)
	if u.CallbackQuery.Data != "us:p:42" {
		t.Errorf("callback data = %q", u.CallbackQuery.Data)
	}
}

func TestTrimUpdate_NilAndEmptyFields(t *testing.T) {
	// Should not panic on nil update or missing fields.
	TrimUpdate(nil)
	TrimUpdate(&tgm.Update{})
	TrimUpdate(&tgm.Update{Message: &tgm.Message{}})
	TrimUpdate(&tgm.Update{CallbackQuery: &tgm.CallbackQuery{}})
}

func TestTrimUpdate_Idempotent(t *testing.T) {
	u := &tgm.Update{Message: &tgm.Message{Text: "already clean"}}
	TrimUpdate(u)
	TrimUpdate(u)
	if u.Message.Text != "already clean" {
		t.Errorf("idempotent trim changed value: %q", u.Message.Text)
	}
}

func TestRecoverHandlerSwallowsPanic(t *testing.T) {
	// Outer deferred recover asserts no panic escapes recoverHandler.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic escaped recoverHandler: %v", r)
		}
	}()
	func() {
		defer recoverHandler(zap.NewNop(), "test")
		panic("boom")
	}()
}

func TestRecoverHandlerNilLogger(t *testing.T) {
	// nil logger must not itself cause a nil-deref panic inside the
	// recover path — recoverHandler falls back to zap.NewNop().
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil-logger recover panicked: %v", r)
		}
	}()
	func() {
		defer recoverHandler(nil, "test")
		panic("also boom")
	}()
}

func TestIsRateLimitExemptText_Exempt(t *testing.T) {
	cases := []string{
		"/cancel",
		"/cancel ",
		"/cancel foo bar",
		"/kill",
		"/kill baz",
		"  /cancel  ",       // leading whitespace
		"/cancel@mybotname", // group-chat disambiguator
		"/cancel@mybotname extra",
		"/kill@mybotname",
	}
	for _, c := range cases {
		if !isRateLimitExemptText(c) {
			t.Errorf("expected exempt: %q", c)
		}
	}
}

func TestIsRateLimitExemptText_NotExempt(t *testing.T) {
	cases := []string{
		"",
		"cancel",     // missing slash
		"/cancelfoo", // prefix without word boundary — bypass target
		"/cancelX",
		"/canceling",
		"/cancel_thing",
		"/killer",
		"/killme",
		"/killswitch",
		"ls -la /tmp",
		"/kill/cancel", // not a real command; /kill\cancel first token is /kill\cancel
		"/get /tmp",    // non-exempt admin command
		"/cancel-x",    // hyphen doesn't form a new command
	}
	for _, c := range cases {
		if isRateLimitExemptText(c) {
			t.Errorf("should NOT be exempt: %q", c)
		}
	}
}
