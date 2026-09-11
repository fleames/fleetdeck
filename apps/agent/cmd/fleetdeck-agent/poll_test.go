package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPollCommandsRequestsLongPoll(t *testing.T) {
	var wait string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wait = r.URL.Query().Get("wait")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	creds := credentials{APIURL: server.URL, PublicID: "agent", Secret: "secret"}
	if err := pollCommands(context.Background(), creds); err != nil {
		t.Fatal(err)
	}
	if wait != commandLongPollWait.String() {
		t.Fatalf("wait=%q want %q", wait, commandLongPollWait)
	}
}

func TestDefaultRecurringRequestBudget(t *testing.T) {
	const day = 24 * 60 * 60
	metrics := day / 10
	heartbeats := day / int(heartbeatInterval.Seconds())
	inventories := day / int(inventoryInterval.Seconds())
	commandPolls := day / int(commandLongPollWait.Seconds())
	total := metrics + heartbeats + inventories + commandPolls
	if total != 16416 {
		t.Fatalf("requests/day=%d want 16416", total)
	}
}
