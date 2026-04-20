package stream

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"

	"github.com/amiwrpremium/shellboto/internal/shell"
	ns "github.com/amiwrpremium/shellboto/internal/telegram/namespaces"
)

const (
	// Telegram's hard text cap is 4096 UTF-16 code units. Reserve a few for
	// the <pre>…</pre> wrapper and a possible exit-footer.
	defaultMaxChars = 4096
	wrapperOverhead = 60 // <pre></pre> + newline + footer cushion

	// Telegram's typing/upload chat action lasts ~5s in the UI; refresh
	// faster so there are no gaps.
	typingRefresh = 4 * time.Second

	liveTrailer = "\n<i>…</i>"
)

// RunningKeyboard is the [Cancel] [Kill] inline keyboard attached to the
// live-streaming message for a running command. Data handled by the
// telegram/callbacks package under ns.Job.
func RunningKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: "↯ Cancel", CallbackData: ns.CBData(ns.Job, ns.JobCancel)},
			{Text: "☠ Kill", CallbackData: ns.CBData(ns.Job, ns.JobKill)},
		}},
	}
}

// emptyKeyboard clears any existing keyboard when edited into a message.
func emptyKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{},
	}
}

type Config struct {
	EditInterval    time.Duration
	MaxMessageChars int
}

type Streamer struct {
	b   *bot.Bot
	cfg Config
	log *zap.Logger
}

func New(b *bot.Bot, cfg Config, log *zap.Logger) *Streamer {
	if log == nil {
		log = zap.NewNop()
	}
	if cfg.EditInterval <= 0 {
		cfg.EditInterval = time.Second
	}
	if cfg.MaxMessageChars <= 0 {
		cfg.MaxMessageChars = defaultMaxChars
	}
	return &Streamer{b: b, cfg: cfg, log: log}
}

// ActionLoop sends `action` immediately, then refreshes every ~4s until ctx
// cancels. Exposed so command handlers can show the "sending a file" status
// during /get and /audit-out transfers.
func (s *Streamer) ActionLoop(ctx context.Context, chatID int64, action models.ChatAction) {
	send := func() {
		_, _ = s.b.SendChatAction(ctx, &bot.SendChatActionParams{
			ChatID: chatID,
			Action: action,
		})
	}
	send()
	t := time.NewTicker(typingRefresh)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			send()
		}
	}
}

// Stream drives the Telegram UI for a single job:
//   - Posts a placeholder message and edits it live every `edit_interval`.
//   - Shows "… is typing" throughout.
//   - When the current message would exceed Telegram's cap, finalizes it at
//     a clean boundary (last `\n`, fallback to space, fallback to hard cut)
//     and rolls to a new message.
//   - On completion, if output spanned more than one message, swaps to the
//     "sending a file" indicator and uploads the full combined output as
//     `output-<unix-ts>.txt`.
func (s *Streamer) Stream(ctx context.Context, chatID int64, j *shell.Job) {
	placeholder, err := s.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "⏳ running…",
		ReplyMarkup: RunningKeyboard(),
	})
	if err != nil {
		s.log.Error("send placeholder", zap.Int64("chat_id", chatID), zap.Error(err))
		return
	}

	typingCtx, cancelTyping := context.WithCancel(ctx)
	go s.ActionLoop(typingCtx, chatID, models.ChatActionTyping)
	defer cancelTyping()

	st := &streamState{
		chatID:       chatID,
		maxChars:     s.cfg.MaxMessageChars,
		currentMsgID: placeholder.ID,
	}

	tick := time.NewTicker(s.cfg.EditInterval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case code, ok := <-j.Done:
			if !ok {
				code = -1
			}
			cancelTyping()
			s.flush(ctx, st, j, true, code, j.Duration())
			if st.multiMessage {
				upCtx, cancelUp := context.WithCancel(ctx)
				go s.ActionLoop(upCtx, chatID, models.ChatActionUploadDocument)
				s.sendFullOutput(ctx, chatID, j, code)
				cancelUp()
			}
			return
		case <-tick.C:
			s.flush(ctx, st, j, false, 0, 0)
		}
	}
}

type streamState struct {
	chatID        int64
	maxChars      int
	currentMsgID  int
	renderedStart int    // offset into job.buf that currentMsg renders from
	lastText      string // previous edit's body; skip API call if unchanged
	multiMessage  bool
}

func (s *Streamer) flush(ctx context.Context, st *streamState, j *shell.Job, final bool, exit int, duration time.Duration) {
	snap, _ := j.Snapshot()
	pending := snap[st.renderedStart:]

	payloadCap := st.maxChars - wrapperOverhead

	// Roll over as many times as needed so that what remains fits one msg.
	for effectiveHTMLLen(pending) > payloadCap {
		cut := pickBreak(pending, payloadCap)
		if cut == 0 {
			break // defensive; pickBreak guarantees >0 when cap>0
		}
		// Finalize the current message: no trailer, clear the keyboard
		// (buttons belong only on the message that's still live).
		s.editMessage(ctx, st, pending[:cut], "", emptyKeyboard())
		s.beginNewMessage(ctx, st)
		st.renderedStart += cut
		pending = pending[cut:]
		st.multiMessage = true
	}

	trailer := liveTrailer
	var kb *models.InlineKeyboardMarkup
	if final {
		footer := finalFooter(exit, duration)
		if j.Truncated() {
			footer += " · ⚠ output capped"
		}
		trailer = "\n" + htmlEscape([]byte(footer))
		kb = emptyKeyboard() // strip buttons once the command's done
	}
	s.editMessage(ctx, st, pending, trailer, kb)
}

func (s *Streamer) editMessage(ctx context.Context, st *streamState, body []byte, trailer string, kb *models.InlineKeyboardMarkup) {
	text := renderBody(body, trailer)
	if text == st.lastText && kb == nil {
		return
	}
	params := &bot.EditMessageTextParams{
		ChatID:    st.chatID,
		MessageID: st.currentMsgID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}
	if kb != nil {
		params.ReplyMarkup = kb
	}
	if _, err := s.b.EditMessageText(ctx, params); err != nil {
		s.log.Debug("edit message", zap.Int("msg_id", st.currentMsgID), zap.Error(err))
		return
	}
	st.lastText = text
}

func (s *Streamer) beginNewMessage(ctx context.Context, st *streamState) {
	msg, err := s.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      st.chatID,
		Text:        "⏳ …",
		ReplyMarkup: RunningKeyboard(),
	})
	if err != nil {
		s.log.Error("send continuation", zap.Error(err))
		return
	}
	st.currentMsgID = msg.ID
	st.lastText = ""
}

func (s *Streamer) sendFullOutput(ctx context.Context, chatID int64, j *shell.Job, exit int) {
	snap, _ := j.Snapshot()
	filename := fmt.Sprintf("output-%d.txt", time.Now().Unix())
	caption := fmt.Sprintf("full output · %d bytes · exit %d", len(snap), exit)
	if _, err := s.b.SendDocument(ctx, &bot.SendDocumentParams{
		ChatID: chatID,
		Document: &models.InputFileUpload{
			Filename: filename,
			Data:     bytes.NewReader(snap),
		},
		Caption: caption,
	}); err != nil {
		s.log.Error("send full output", zap.Error(err))
	}
}

// pickBreak returns the largest prefix length of b that, when HTML-wrapped,
// fits in payloadCap AND ends at a natural boundary (last '\n',
// fallback to last ' ', fallback to hard cut).
func pickBreak(b []byte, payloadCap int) int {
	maxByFit := maxPrefixFitting(b, payloadCap)
	if maxByFit <= 0 {
		return 0
	}
	if i := bytes.LastIndexByte(b[:maxByFit], '\n'); i > 0 {
		return i + 1
	}
	if i := bytes.LastIndexByte(b[:maxByFit], ' '); i > 0 {
		return i + 1
	}
	return maxByFit
}

// maxPrefixFitting returns the largest n such that htmlEscapeLen(b[:n]) <= cap.
func maxPrefixFitting(b []byte, cap int) int {
	running := 0
	for i, c := range b {
		cost := 1
		switch c {
		case '&':
			cost = 5
		case '<', '>':
			cost = 4
		}
		if running+cost > cap {
			return i
		}
		running += cost
	}
	return len(b)
}

func effectiveHTMLLen(b []byte) int {
	n := 0
	for _, c := range b {
		switch c {
		case '&':
			n += 5
		case '<', '>':
			n += 4
		default:
			n++
		}
	}
	return n
}

// renderBody builds the full Telegram HTML text for one message.
// body is raw bytes wrapped in <pre>; trailer is a pre-formatted HTML-safe
// snippet (e.g. "\n<i>…</i>" or an escaped exit footer). If both are empty
// it emits a placeholder.
func renderBody(body []byte, trailer string) string {
	if len(body) == 0 && trailer == "" {
		return "⏳ running…"
	}
	var sb strings.Builder
	sb.WriteString("<pre>")
	sb.WriteString(htmlEscape(body))
	sb.WriteString("</pre>")
	sb.WriteString(trailer)
	return sb.String()
}

func finalFooter(exit int, duration time.Duration) string {
	d := duration.Round(10 * time.Millisecond)
	switch {
	case exit == 0:
		return fmt.Sprintf("✅ exit 0 · %s", d)
	case exit == -1:
		return fmt.Sprintf("⚠ shell died · %s", d)
	default:
		return fmt.Sprintf("❌ exit %d · %s", exit, d)
	}
}

var htmlReplacer = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func htmlEscape[T []byte | string](v T) string {
	return htmlReplacer.Replace(string(v))
}
