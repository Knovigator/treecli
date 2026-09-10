package recorder

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestMain doubles as a fake memdb-rust binary: tests point Memdb.Binary at
// the test executable with TREECLI_FAKE_MEMDB=1, so no shell script and no
// real memdb are needed. Every invocation is appended to
// TREECLI_FAKE_MEMDB_LOG as one JSON line for assertions.
func TestMain(m *testing.M) {
	if os.Getenv("TREECLI_FAKE_MEMDB") == "1" {
		os.Exit(runFakeMemdb(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func runFakeMemdb(args []string) int {
	if logPath := os.Getenv("TREECLI_FAKE_MEMDB_LOG"); logPath != "" {
		file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			encoded, _ := json.Marshal(args)
			fmt.Fprintln(file, string(encoded))
			file.Close()
		}
	}
	if os.Getenv("TREECLI_FAKE_MEMDB_FAIL") == "1" {
		fmt.Fprintln(os.Stderr, "fake memdb failure")
		return 1
	}
	command := ""
	for index := 0; index < len(args); index++ {
		if args[index] == "--db" {
			index++
			continue
		}
		command = args[index]
		break
	}
	switch command {
	case "init":
		return 0
	case "ingest":
		messages, events := 0, 0
		for _, arg := range args {
			if strings.HasSuffix(arg, ".jsonl") {
				data, err := os.ReadFile(arg)
				if err != nil {
					fmt.Fprintln(os.Stderr, "fake memdb: cannot read", arg)
					return 1
				}
				for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
					if strings.Contains(line, `"type":"message"`) {
						messages++
					} else if line != "" {
						events++
					}
				}
			}
		}
		fmt.Printf(`{"files":1,"messages_added":%d,"events_added":%d}`+"\n", messages, events)
	case "recall":
		if canned := os.Getenv("TREECLI_FAKE_MEMDB_RECALL_JSON"); canned != "" {
			fmt.Println(canned)
		} else {
			fmt.Println(`{"mode":"thread","query":null,"scope":{},"thread":{"recent":[],"query":[]},"long_term":null}`)
		}
	case "recent":
		if canned := os.Getenv("TREECLI_FAKE_MEMDB_RECENT_JSON"); canned != "" {
			fmt.Println(canned)
		} else {
			fmt.Println(`{"scope":{},"results":[]}`)
		}
	case "query":
		if canned := os.Getenv("TREECLI_FAKE_MEMDB_QUERY_JSON"); canned != "" {
			fmt.Println(canned)
		} else {
			fmt.Println(`{"query":"x","scope":{},"results":[]}`)
		}
	case "learn":
		fmt.Println(`{"id":7,"kind":"learned","memory_type":"decision","key":"fake","title":"fake","source_file":"learned://fake"}`)
	default:
		fmt.Fprintln(os.Stderr, "fake memdb: unknown command", command)
		return 2
	}
	return 0
}

// fakeMemdb returns a Memdb backed by the test binary plus the log path it writes to.
func fakeMemdb(t *testing.T, extraEnv ...string) (*Memdb, string) {
	t.Helper()
	dir := t.TempDir()
	logPath := dir + "/memdb-calls.log"
	env := append([]string{"TREECLI_FAKE_MEMDB=1", "TREECLI_FAKE_MEMDB_LOG=" + logPath}, extraEnv...)
	return &Memdb{Binary: os.Args[0], DBPath: dir + "/memdb.sqlite3", Env: env}, logPath
}

func fakeMemdbCalls(t *testing.T, logPath string) [][]string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("reading fake memdb log: %v", err)
	}
	calls := [][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var call []string
		if err := json.Unmarshal([]byte(line), &call); err != nil {
			t.Fatalf("corrupt fake memdb log line %q: %v", line, err)
		}
		calls = append(calls, call)
	}
	return calls
}

func hasCall(calls [][]string, command string, contains ...string) bool {
	for _, call := range calls {
		joined := " " + strings.Join(call, " ") + " "
		if !strings.Contains(joined, " "+command+" ") {
			continue
		}
		matched := true
		for _, needle := range contains {
			if !strings.Contains(joined, needle) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
