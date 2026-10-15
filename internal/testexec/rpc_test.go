package testexec_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/eosaios/eos/internal/testexec"
	"github.com/eosaios/eos/pkg/protocol/jsonrpc"
)

func TestRPCPeerFramingUnicodeAndErrors(t *testing.T) {
	binary := testexec.RPCServer(t, []string{"session/list"})
	for _, mode := range []string{"ok", "error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, mode)
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := stdin.Close(); err != nil {
					t.Error(err)
				}
				if err := cmd.Wait(); err != nil {
					t.Error(err)
				}
			}()
			stream := jsonrpc.NewStream(stdout, stdin)
			if err := stream.WriteFrame([]byte(`{"jsonrpc":"2.0","id":1,"method":"turn/start","params":{"session_id":"会话","turn_id":"t1"}}`)); err != nil {
				t.Fatal(err)
			}
			expected := []string{
				`{"jsonrpc":"2.0","id":1,"result":{"id":"t1","session_id":"会话","status":"running"}}`,
				`{"jsonrpc":"2.0","method":"event","params":{"event_type":"turn.item_started","session_id":"会话","turn_id":"t1","source":"runtime","payload":{"item_id":"it-1"}}}`,
				`{"jsonrpc":"2.0","method":"event","params":{"event_type":"turn.item_delta","session_id":"会话","turn_id":"t1","source":"runtime","payload":{"delta":"你好，","delta_type":"text"}}}`,
				`{"jsonrpc":"2.0","method":"event","params":{"event_type":"turn.item_delta","session_id":"会话","turn_id":"t1","source":"runtime","payload":{"delta":"世界","delta_type":"text"}}}`,
				`{"jsonrpc":"2.0","method":"event","params":{"event_type":"turn.completed","session_id":"会话","turn_id":"t1","source":"runtime","payload":{}}}`,
			}
			if mode == "error" {
				expected = []string{`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"kernel stub failure"}}`}
			}
			for _, wantJSON := range expected {
				frame, err := stream.ReadFrame()
				if err != nil {
					t.Fatal(err)
				}
				var got, want any
				if err := json.Unmarshal(frame, &got); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("frame = %s, want %s", frame, wantJSON)
				}
			}
		})
	}
}
