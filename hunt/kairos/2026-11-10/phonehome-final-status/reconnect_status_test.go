package phonehome_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kairos-io/kairos/v4/agent/internal/phonehome"
)

// First WS session: server sends a command, then drops the connection.
// Handler finishes after the agent has reconnected (second session).
// Expect: Completed status reaches server on some connection.
func TestFinalStatusAfterReconnect(t *testing.T) {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	var mu sync.Mutex
	sessions := 0
	var statuses []string
	secondUp := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		mu.Lock()
		sessions++
		n := sessions
		mu.Unlock()
		if n == 1 {
			data, _ := json.Marshal(phonehome.CommandData{ID: "cmd-1", Command: "exec"})
			b, _ := json.Marshal(phonehome.WSMessage{Type: "command", Data: data})
			conn.WriteMessage(websocket.TextMessage, b)
			// wait for Running, then drop session (server restart / LB idle / network blip)
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				_, m, err := conn.ReadMessage()
				if err != nil {
					return
				}
				var msg phonehome.WSMessage
				json.Unmarshal(m, &msg)
				if msg.Type == "command_status" {
					var s phonehome.CommandStatusData
					json.Unmarshal(msg.Data, &s)
					mu.Lock()
					statuses = append(statuses, "s1:"+s.Phase)
					mu.Unlock()
					break
				}
			}
			return // closes conn abruptly
		}
		if n == 2 {
			close(secondUp)
		}
		for {
			_, m, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg phonehome.WSMessage
			json.Unmarshal(m, &msg)
			if msg.Type == "command_status" {
				var s phonehome.CommandStatusData
				json.Unmarshal(msg.Data, &s)
				mu.Lock()
				statuses = append(statuses, "s2:"+s.Phase)
				mu.Unlock()
			}
		}
	})
	mux.HandleFunc("/api/v1/nodes/register", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(phonehome.Credentials{NodeID: "n", APIKey: "k"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := &phonehome.Config{URL: srv.URL, RegistrationToken: "<redacted>",
		HeartbeatInterval: 100 * time.Millisecond, ReconnectBackoff: 50 * time.Millisecond}
	c := phonehome.NewClient(cfg,
		phonehome.WithCredentialsPath(filepath.Join(t.TempDir(), "c.yaml")),
		phonehome.WithMachineIDFunc(func() string { return "m" }),
		phonehome.WithCommandHandler(func(cmd phonehome.CommandData) (string, error) {
			<-secondUp                    // long-running command outlives first session
			time.Sleep(200 * time.Millisecond)
			return "done", nil
		}))
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	go c.Run(ctx)
	time.Sleep(3 * time.Second)

	mu.Lock()
	defer mu.Unlock()
	t.Logf("sessions=%d statuses=%v", sessions, statuses)
	for _, s := range statuses {
		if s == "s2:Completed" || s == "s1:Completed" {
			return
		}
	}
	t.Fatalf("final Completed status never reached server; got %v", statuses)
}
