package webbridge

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/eosaios/eos/internal/testexec"
)

func installOpenCommandFixtures(t *testing.T) (string, string) {
	t.Helper()
	calls := filepath.Join(t.TempDir(), "calls")
	program := testexec.Build(t, "open", fmt.Sprintf(`package main
import("fmt";"os";"strings")
func main(){f,err:=os.OpenFile(%q,os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);if err!=nil{panic(err)};defer f.Close();fmt.Fprintln(f,os.Args[0]+" "+strings.Join(os.Args[1:]," "))}
`, calls))
	data, err := os.ReadFile(program)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(program)
	for _, name := range []string{"xdg-open", "rundll32", "cmd", "explorer", "wt", "x-terminal-emulator"} {
		if err := os.WriteFile(filepath.Join(dir, testexec.Name(name)), data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir, calls
}
