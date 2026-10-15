package testexec

import (
	"encoding/json"
	"fmt"
	"testing"
)

// RPCServer creates a framed JSON-RPC peer for process and CLI integration tests.
// An "error" process argument makes requests fail; other arguments are ignored.
func RPCServer(t testing.TB, methods []string) string {
	t.Helper()
	encoded, err := json.Marshal(methods)
	if err != nil {
		t.Fatal(err)
	}
	return Build(t, "eos-core", fmt.Sprintf(rpcSource, string(encoded)))
}

const rpcSource = `package main
import("bufio";"encoding/json";"fmt";"io";"os";"strconv";"strings")
func send(value any){data,err:=json.Marshal(value);if err!=nil{panic(err)};fmt.Fprintf(os.Stdout,"Content-Length: %%d\r\n\r\n",len(data));os.Stdout.Write(data)}
func notify(kind,sid,tid string,payload any){send(map[string]any{"jsonrpc":"2.0","method":"event","params":map[string]any{"event_type":kind,"session_id":sid,"turn_id":tid,"source":"runtime","payload":payload}})}
func main(){
 var methods []string
 if err:=json.Unmarshal([]byte(%q),&methods);err!=nil{panic(err)}
 reader:=bufio.NewReader(os.Stdin)
 for {
  length:=0
  for {line,err:=reader.ReadString('\n');if err!=nil{return};line=strings.TrimSpace(line);if line==""{break};key,value,ok:=strings.Cut(line,":");if ok && strings.EqualFold(key,"Content-Length"){length,_=strconv.Atoi(strings.TrimSpace(value))}}
  if length<=0{return};data:=make([]byte,length);if _,err:=io.ReadFull(reader,data);err!=nil{return}
  var req struct{ID any;Method string;Params map[string]any};if err:=json.Unmarshal(data,&req);err!=nil{panic(err)}
  response:=map[string]any{"jsonrpc":"2.0","id":req.ID}
  if len(os.Args)>1 && os.Args[1]=="error" {response["error"]=map[string]any{"code":-32000,"message":"kernel stub failure"};send(response);continue}
  result:=map[string]any{}
  sid:="s-stub";if value,ok:=req.Params["session_id"].(string);ok{sid=value}
  tid:="t-stub";if value,ok:=req.Params["turn_id"].(string);ok{tid=value}
  cwd,_:=os.Getwd()
  switch req.Method{
  case "initialize":result=map[string]any{"server_name":"stub-core","protocol_version":"1","methods":methods}
  case "session/current":result=map[string]any{"id":"","workspace_root":cwd}
  case "session/create","session/resume":result=map[string]any{"id":sid,"workspace_root":cwd}
  case "turn/start":result=map[string]any{"id":tid,"session_id":sid,"status":"running"}
  case "event/subscribe":result=map[string]any{"subscription_id":"sub-stub"}
  }
  response["result"]=result;send(response)
  if req.Method=="turn/start"{notify("turn.item_started",sid,tid,map[string]any{"item_id":"it-1"});notify("turn.item_delta",sid,tid,map[string]any{"delta":"你好，","delta_type":"text"});notify("turn.item_delta",sid,tid,map[string]any{"delta":"世界","delta_type":"text"});notify("turn.completed",sid,tid,map[string]any{})}
 }
}
`
