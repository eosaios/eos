package testexec_test

import (
	"os/exec"
	"testing"

	"github.com/eosaios/eos/internal/testexec"
)

func TestBuildDoesNotDependOnScenarioHomeOrPath(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("PATH", t.TempDir())
	binary := testexec.Build(t, "native-probe", `package main
import "fmt"
func main(){fmt.Print("native-ready")}
`)
	output, err := exec.Command(binary).CombinedOutput()
	if err != nil || string(output) != "native-ready" {
		t.Fatalf("native fixture = %q, %v", output, err)
	}
}
