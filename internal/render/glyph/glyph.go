package glyph

import (
	"os"

	vk "github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/signals"
)

type Mode = vk.Mode

const (
	ModeNerd    = vk.ModeNerd
	ModeUnicode = vk.ModeUnicode
	ModeNone    = vk.ModeNone
)

func SetMode(m Mode) { vk.SetMode(m) }

func CurrentMode() Mode { return vk.CurrentMode() }

func Resolve() { vk.Detect(os.Stdout, os.Getenv("MUNIN_ICONS")) }

func Pad(s string) string { return vk.Pad(s) }

func Lead(s string) string { return vk.Lead(s) }

func StatusOK() string    { return vk.StatusOK() }
func StatusWarn() string  { return vk.StatusWarn() }
func StatusBad() string   { return vk.StatusBad() }
func StatusMuted() string { return vk.StatusMuted() }
func Check() string       { return vk.Check() }
func Cross() string       { return vk.Cross() }
func Warn() string        { return vk.Warn() }
func Arrow() string       { return vk.Arrow() }
func Bullet() string      { return vk.Bullet() }
func GitHub() string      { return vk.GitHub() }
func Slack() string       { return vk.Slack() }
func Google() string      { return vk.Google() }
func Flight() string      { return vk.Diamond() }
func History() string     { return vk.History() }
func Directives() string  { return vk.List() }
func Audit() string       { return vk.Database() }
func Settings() string    { return vk.Cog() }
func Role() string        { return vk.User() }
func Quit() string        { return vk.SignOut() }
func Clock() string       { return vk.Clock() }
func Notes() string       { return notes.String() }
func Plugins() string     { return plugins.String() }
func Builder() string     { return builder.String() }

var (
	brand      = vk.Variants{Nerd: "▚▚", Uni: "▚▚", ASCII: "##"}
	signingOK  = vk.Variants{Nerd: "", Uni: "✓", ASCII: "ok"}
	signingBad = vk.Variants{Nerd: "", Uni: "✗", ASCII: "x"}
	login      = vk.Variants{Nerd: "", Uni: "⚷", ASCII: ">"}
	notes      = vk.Variants{Nerd: "", Uni: "✎", ASCII: "nt"}
	plugins    = vk.Variants{Nerd: "", Uni: "▣", ASCII: "P"}
	builder    = vk.Variants{Nerd: "", Uni: "⚒", ASCII: "qb"}
)

func init() {
	vk.Register("notes", notes)
	vk.Register("ntr", notes)
	vk.Register("plugins", plugins)
	vk.Register("builder", builder)
}

func Brand() string      { return brand.String() }
func SigningOK() string  { return signingOK.String() }
func SigningBad() string { return signingBad.String() }
func Login() string      { return login.String() }

func ForTool(name string) string { return vk.ResolveID(name) }

type Kind = vk.Severity

const (
	KindNeutral  = vk.SeverityNeutral
	KindPositive = vk.SeverityPositive
	KindWarning  = vk.SeverityWarning
	KindNegative = vk.SeverityNegative
)

func Classify(kind string) Kind { return signals.ClassifyKind(kind) }

func ForKind(kind string) string { return vk.GlyphFor(signals.ClassifyKind(kind)) }
