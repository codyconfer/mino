package plugin

import (
	"sort"
	"sync"
)

type ParamSpec struct {
	Key     string
	Desc    string
	Example string
	Values  []string
	Delim   string
}

var (
	paramMu    sync.RWMutex
	paramSpecs = map[string][]ParamSpec{}
)

func RegisterQueryParams(signal string, specs ...ParamSpec) {
	if signal == "" || len(specs) == 0 {
		return
	}
	paramMu.Lock()
	defer paramMu.Unlock()
	if _, dup := paramSpecs[signal]; dup {
		return
	}
	paramSpecs[signal] = append([]ParamSpec(nil), specs...)
}

func QueryParams(signal string) []ParamSpec {
	paramMu.RLock()
	defer paramMu.RUnlock()
	specs, ok := paramSpecs[signal]
	if !ok {
		return nil
	}
	out := make([]ParamSpec, len(specs))
	copy(out, specs)
	return out
}

func ParamSignals() []string {
	paramMu.RLock()
	out := make([]string, 0, len(paramSpecs))
	for name := range paramSpecs {
		out = append(out, name)
	}
	paramMu.RUnlock()
	sort.Strings(out)
	return out
}

func ResetQueryParams() {
	paramMu.Lock()
	paramSpecs = map[string][]ParamSpec{}
	paramMu.Unlock()
}
