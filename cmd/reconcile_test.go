package cmd

import (
	"testing"

	"github.com/codyconfer/sisyphus"
)

func TestReconcileResolver(t *testing.T) {
	tests := []struct {
		name     string
		resolver reconcileResolver
		rec      sisyphus.Reconciliation
		want     sisyphus.Action
	}{
		{
			name:     "no db version imports even when interactive",
			resolver: reconcileResolver{interactive: true},
			rec:      sisyphus.Reconciliation{Name: "config", HasDB: false},
			want:     sisyphus.ActionImport,
		},
		{
			name:     "no db version imports non-interactive",
			resolver: reconcileResolver{interactive: false},
			rec:      sisyphus.Reconciliation{Name: "config", HasDB: false},
			want:     sisyphus.ActionImport,
		},
		{
			name:     "db conflict non-interactive prefers db",
			resolver: reconcileResolver{interactive: false, preferDB: true},
			rec:      sisyphus.Reconciliation{Name: "config", HasDB: true},
			want:     sisyphus.ActionUseDB,
		},
		{
			name:     "db conflict non-interactive uses file by default",
			resolver: reconcileResolver{interactive: false},
			rec:      sisyphus.Reconciliation{Name: "config", HasDB: true},
			want:     sisyphus.ActionUseFile,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.resolver.Resolve(tt.rec)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Resolve = %v, want %v", got, tt.want)
			}
		})
	}
}
