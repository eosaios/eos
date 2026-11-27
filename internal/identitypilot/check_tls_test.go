package identitypilot

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureCAIsRestrictedToExplicitLoopbackOrigins(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	caFile := filepath.Join(t.TempDir(), "server.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caFile, pemBytes, 0600); err != nil {
		t.Fatal(err)
	}
	settings := Settings{Issuer: server.URL, APIOrigin: server.URL, CAFile: caFile}
	transport, err := loopbackTransport(settings)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("HTTPS unavailable through fixture CA, got %d", response.StatusCode)
	}
	for _, remote := range []Settings{
		{Issuer: "https://issuer.example.test", APIOrigin: server.URL, CAFile: caFile},
		{Issuer: server.URL, APIOrigin: "https://api.example.test", CAFile: caFile},
		{Issuer: "http://127.0.0.1", APIOrigin: server.URL, CAFile: caFile},
		{Issuer: server.URL, APIOrigin: server.URL, CAFile: caFile + ".missing"},
	} {
		if _, err := loopbackTransport(remote); err != ErrConfig {
			t.Fatalf("fixture configuration should have been refused, got %v", err)
		}
	}
}
