package glyph

import (
	"os"
	"strings"

	vk "github.com/codyconfer/viewkit/glyph"
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
func Flight() string      { return vk.Flight() }
func History() string     { return vk.History() }
func Directives() string  { return vk.List() }
func Audit() string       { return vk.Database() }
func Settings() string    { return vk.Cog() }
func Role() string        { return vk.User() }
func Quit() string        { return vk.SignOut() }
func Clock() string       { return vk.Clock() }

var (
	brand      = vk.Variants{Nerd: "▚▚", Uni: "▚▚", ASCII: "##"}
	signingOK  = vk.Variants{Nerd: "", Uni: "✓", ASCII: "ok"}
	signingBad = vk.Variants{Nerd: "", Uni: "✗", ASCII: "x"}
	login      = vk.Variants{Nerd: "", Uni: "⚷", ASCII: ">"}
)

func Brand() string      { return brand.String() }
func SigningOK() string  { return signingOK.String() }
func SigningBad() string { return signingBad.String() }
func Login() string      { return login.String() }

type Kind int

const (
	KindNeutral Kind = iota
	KindPositive
	KindWarning
)

func Classify(kind string) Kind {
	switch strings.ToLower(kind) {
	case "mention", "review-requested", "review_requested", "assigned", "alert", "incident":
		return KindWarning
	case "merged", "approved", "completed", "resolved", "success", "closed":
		return KindPositive
	}
	return KindNeutral
}

func ForKind(kind string) string {
	switch Classify(kind) {
	case KindPositive:
		return vk.Check()
	case KindWarning:
		return vk.Warn()
	}
	return vk.Bullet()
}
