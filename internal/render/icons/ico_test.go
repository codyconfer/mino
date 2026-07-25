package icons

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPngInICO(t *testing.T) {
	pngPath := filepath.Join("data", "dark", "running.png")
	png, err := os.ReadFile(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	ico, err := pngInICO(png)
	if err != nil {
		t.Fatal(err)
	}
	if !isICO(ico) {
		t.Fatal("result is not an ICO")
	}
	if len(ico) != 22+len(png) {
		t.Fatalf("ico len = %d, want %d", len(ico), 22+len(png))
	}
	if !isPNG(ico[22:]) {
		t.Fatal("ICO payload is not the original PNG")
	}
}

func TestPngInICORejectsNonPNG(t *testing.T) {
	if _, err := pngInICO([]byte("not-a-png")); err == nil {
		t.Fatal("expected error")
	}
}
