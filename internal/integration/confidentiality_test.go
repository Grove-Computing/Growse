package integration_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
	"github.com/Grove-Computing/Growse/internal/webapi/navigation"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (transport transportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestStorageHistoryAndCredentialsAreAbsentFromLogsAndErrors(t *testing.T) {
	secrets := []string{"storage-value-secret", "history-state-secret", "alice", "password", "bearer-secret", "session-secret"}
	area := storagecore.NewArea()
	storageErr := area.Set(strings.Repeat("storage-value-secret", storagecore.MaxKeyBytes), "value")
	if storageErr == nil {
		t.Fatal("oversized Storage key error = nil")
	}
	navigationAPI := navigation.New(parseURL(t, "https://example.test/page"))
	historyErr := navigationAPI.PushState("history-state-secret", "/private")
	if historyErr == nil {
		t.Fatal("invalid History state error = nil")
	}

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	navigationAPI.OnPopState(func(event navigation.PopStateEvent) {
		panic("history callback " + event.State)
	})
	navigationAPI.DispatchPopState("history-state-secret")

	sentinel := errors.New("transport failed")
	client := network.NewClientWithLimits(&http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("%w url=%s authorization=%s cookie=%s", sentinel, request.URL, request.Header.Get("Authorization"), request.Header.Get("Cookie"))
	})}, 1024)
	_, networkErr := client.Do(context.Background(), &network.Request{
		URL: parseURL(t, "https://alice:password@example.test/private"),
		Header: http.Header{
			"Authorization": []string{"Bearer bearer-secret"}, "Cookie": []string{"session=session-secret"},
		},
	})
	if !errors.Is(networkErr, sentinel) {
		t.Fatalf("Network error chain = %v", networkErr)
	}
	combined := storageErr.Error() + "\n" + historyErr.Error() + "\n" + networkErr.Error() + "\n" + logs.String()
	for _, secret := range secrets {
		if strings.Contains(combined, secret) {
			t.Fatalf("Log/Error exposed %q: %s", secret, combined)
		}
	}
}
