package generated

import (
	"sort"
	"testing"

	"github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

func TestGeneratedMethodsCoverAllCoreMethods(t *testing.T) {
	expected := jsonrpc.AllCoreMethods()
	got := CoreMethods()

	sort.Strings(expected)
	sort.Strings(got)

	expectedSet := make(map[string]bool, len(expected))
	for _, m := range expected {
		expectedSet[m] = true
	}

	gotSet := make(map[string]bool, len(got))
	for _, m := range got {
		gotSet[m] = true
	}

	for _, m := range expected {
		if !gotSet[m] {
			t.Errorf("CoreMethods() missing method %q from AllCoreMethods()", m)
		}
	}

	for _, m := range got {
		if !expectedSet[m] {
			t.Errorf("CoreMethods() has extra method %q not in AllCoreMethods()", m)
		}
	}
}
