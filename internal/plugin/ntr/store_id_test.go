package ntr

import (
	"errors"
	"strings"
	"testing"
)

func TestIDFromRowsEmptyReturnsSentinel(t *testing.T) {
	_, err := idFromRows(nil)
	if err == nil {
		t.Fatal("empty id sequence result returned no error")
	}
	if !errors.Is(err, ErrNoID) {
		t.Fatalf("err = %v, want one matching ErrNoID", err)
	}
	if strings.Contains(err.Error(), "%!w") {
		t.Fatalf("error formats a nil cause with %%w: %q", err.Error())
	}

	if _, err := idFromRows([][]string{{}}); !errors.Is(err, ErrNoID) {
		t.Fatalf("empty row = %v, want ErrNoID", err)
	}
}

func TestIDFromRowsParses(t *testing.T) {
	id, err := idFromRows([][]string{{"42"}})
	if err != nil || id != 42 {
		t.Fatalf("idFromRows = (%d, %v), want (42, nil)", id, err)
	}
	if _, err := idFromRows([][]string{{"not-a-number"}}); err == nil || errors.Is(err, ErrNoID) {
		t.Fatalf("bad id = %v, want a parse error distinct from ErrNoID", err)
	}
}
