package cmd

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/onboard"
)

func isolateSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
}

func TestEnforceOnboardingBlocksNormalCommand(t *testing.T) {
	isolateSettings(t)
	c := &cobra.Command{Use: "fly", RunE: func(*cobra.Command, []string) error { return nil }}
	err := enforceOnboarding(c)
	if err == nil {
		t.Fatal("expected onboarding error for a normal command before onboarding")
	}
	if errs.KindOf(err) != errs.KindOnboarding {
		t.Fatalf("expected KindOnboarding, got %v", errs.KindOf(err))
	}
}

func TestEnforceOnboardingAllowsAnnotatedCommand(t *testing.T) {
	isolateSettings(t)
	c := &cobra.Command{
		Use:         "onboard",
		Annotations: map[string]string{annoSkipOnboarding: "true"},
		RunE:        func(*cobra.Command, []string) error { return nil },
	}
	if err := enforceOnboarding(c); err != nil {
		t.Fatalf("annotated command should be allowed, got %v", err)
	}
}

func TestEnforceOnboardingAllowsNonRunnableParent(t *testing.T) {
	isolateSettings(t)
	c := &cobra.Command{Use: "drive"}
	if err := enforceOnboarding(c); err != nil {
		t.Fatalf("non-runnable parent should be allowed, got %v", err)
	}
}

func TestEnforceOnboardingAllowsWhenOnboarded(t *testing.T) {
	isolateSettings(t)
	if err := config.SaveGlobalSettings(config.GlobalSettings{Onboarded: true}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	c := &cobra.Command{Use: "fly", RunE: func(*cobra.Command, []string) error { return nil }}
	if err := enforceOnboarding(c); err != nil {
		t.Fatalf("expected allow after onboarding, got %v", err)
	}
}

func TestEnforceOnboardingRestrictedRejectsForeignMarker(t *testing.T) {
	isolateSettings(t)
	orig := onboard.RequiredEmailDomain
	onboard.RequiredEmailDomain = "example.com"
	t.Cleanup(func() { onboard.RequiredEmailDomain = orig })

	if err := config.SaveGlobalSettings(config.GlobalSettings{Onboarded: true}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	c := &cobra.Command{Use: "fly", RunE: func(*cobra.Command, []string) error { return nil }}
	if err := enforceOnboarding(c); err == nil {
		t.Fatal("restricted build must reject a marker onboarded without the required domain")
	}

	if err := config.SaveGlobalSettings(config.GlobalSettings{Onboarded: true, OnboardedDomain: "example.com"}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := enforceOnboarding(c); err != nil {
		t.Fatalf("expected allow when marker domain matches, got %v", err)
	}
}
