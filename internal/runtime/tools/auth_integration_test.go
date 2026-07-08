package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildHTTPHeaders_WIFAndStatic(t *testing.T) {
	cases := []struct {
		name, expect string
		cfg          *HTTPCfg
		exec         *OmniaExecutor
	}{
		{"none", "", &HTTPCfg{}, &OmniaExecutor{}},
		{"bearer", "Bearer btok", &HTTPCfg{AuthType: "bearer", AuthToken: "btok"}, &OmniaExecutor{}},
		{"wif", "Bearer wtok", &HTTPCfg{AuthType: "workloadIdentity", AuthCloud: "azure", AuthAudience: "api://tool"},
			&OmniaExecutor{tokenAcquirer: fakeAcquirer{tok: "wtok"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.Header.Get("Authorization") }))
			defer srv.Close()
			headers, err := tc.exec.buildHTTPHeaders(context.Background(), tc.cfg, "tool", "h", nil)
			if err != nil {
				t.Fatalf("buildHTTPHeaders: %v", err)
			}
			req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			if _, err := http.DefaultClient.Do(req); err != nil {
				t.Fatal(err)
			}
			if got != tc.expect {
				t.Fatalf("Authorization = %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestBuildHTTPHeaders_WIFFailsLoud(t *testing.T) {
	exec := &OmniaExecutor{}
	cfg := &HTTPCfg{AuthType: "workloadIdentity", AuthCloud: "azure", AuthAudience: "api://tool"}
	if _, err := exec.buildHTTPHeaders(context.Background(), cfg, "tool", "h", nil); err == nil {
		t.Fatal("expected error when no tokenAcquirer is configured")
	}
}
