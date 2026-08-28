package devtools

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
)

func TestNetworkRecordRedactsCredentialsAndQueryValues(t *testing.T) {
	store := NewPageStore()
	target, err := url.Parse("https://user:password@example.test/private?token=secret&name=alice#fragment")
	if err != nil {
		t.Fatal(err)
	}
	store.ObserveNetwork(network.Observation{
		Method: "get", URL: target, Kind: network.RequestFetch, StartedAt: time.Unix(10, 0), Duration: time.Second,
		StatusCode: 200, CacheStatus: "hit", ResponseBytes: 42,
	})
	records := store.Network()
	if len(records) != 1 {
		t.Fatalf("records = %+v", records)
	}
	record := records[0]
	if strings.Contains(record.URL, "user") || strings.Contains(record.URL, "password") || strings.Contains(record.URL, "secret") || strings.Contains(record.URL, "alice") || strings.Contains(record.URL, "fragment") {
		t.Fatalf("credential remained in URL: %q", record.URL)
	}
	if record.URL != "https://example.test/private?name=%5BREDACTED%5D&token=%5BREDACTED%5D" || record.Kind != "fetch" || record.Method != "GET" {
		t.Fatalf("record = %+v", record)
	}
}

func TestNetworkRecordRetainsOnlyScriptEngineMetadata(t *testing.T) {
	store := NewPageStore()
	store.ObserveNetwork(network.Observation{
		Method: "GET", URL: &url.URL{Scheme: "https", Host: "example.test", Path: "/app.js"},
		Kind: network.RequestScript, Engine: "javascript",
	})
	records := store.Network()
	if len(records) != 1 || records[0].Kind != "script" || records[0].Engine != "javascript" {
		t.Fatalf("Network() = %#v, want JavaScript script metadata", records)
	}
}

func TestNetworkRecordPageSessionAndLifecycleLimits(t *testing.T) {
	session := &SessionStore{max: 3}
	first := NewPageStoreForSession(session)
	second := NewPageStoreForSession(session)
	first.maxNetwork = 2
	second.maxNetwork = 2
	observation := network.Observation{Method: "GET", URL: &url.URL{Scheme: "https", Host: "example.test"}}
	for range 3 {
		first.ObserveNetwork(observation)
	}
	for range 3 {
		second.ObserveNetwork(observation)
	}
	if got := len(first.Network()); got != 2 {
		t.Fatalf("first page records = %d, want 2", got)
	}
	if got := len(second.Network()); got != 1 {
		t.Fatalf("second page records = %d, want shared session remainder 1", got)
	}
	second.Close()
	second.ObserveNetwork(observation)
	if got := len(second.Network()); got != 0 {
		t.Fatalf("closed page records = %d, want 0", got)
	}
}

func TestNetworkRecordProductionPageLimit(t *testing.T) {
	store := NewPageStore()
	observation := network.Observation{Method: "GET", URL: &url.URL{Scheme: "https", Host: "example.test"}}
	for range DefaultMaxNetworkRecords + 5 {
		store.ObserveNetwork(observation)
	}
	if got := len(store.Network()); got != DefaultMaxNetworkRecords {
		t.Fatalf("page records = %d, want %d", got, DefaultMaxNetworkRecords)
	}
}

func TestNetworkRecordProductionSessionLimit(t *testing.T) {
	session := NewSessionStore()
	observation := network.Observation{Method: "GET", URL: &url.URL{Scheme: "https", Host: "example.test"}}
	total := 0
	for range 9 {
		store := NewPageStoreForSession(session)
		for range DefaultMaxNetworkRecords {
			store.ObserveNetwork(observation)
		}
		total += len(store.Network())
	}
	if total != DefaultMaxSessionNetworkRecords {
		t.Fatalf("session records = %d, want %d", total, DefaultMaxSessionNetworkRecords)
	}
}

func TestNetworkRecordBoundsURLsAndRedactsLocalPaths(t *testing.T) {
	store := NewPageStore()
	longURL, _ := url.Parse("https://example.test/" + strings.Repeat("x", DefaultMaxMessageBytes*2) + "?token=secret")
	store.ObserveNetwork(network.Observation{Method: "GET", URL: longURL, FinalURL: &url.URL{Scheme: "file", Path: "/home/user/private/font.woff2"}})
	records := store.Network()
	if len(records) != 1 || len(records[0].URL) > DefaultMaxMessageBytes || records[0].FinalURL != "[LOCAL_PATH_REDACTED]" {
		t.Fatalf("bounded/redacted Network record = %+v", records)
	}
	if strings.Contains(fmt.Sprint(records), "secret") || strings.Contains(fmt.Sprint(records), "/home/user") {
		t.Fatalf("Network record leaked credential/local path: %+v", records)
	}
}
