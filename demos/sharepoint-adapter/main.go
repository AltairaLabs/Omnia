// Command sharepoint-adapter is a demo-only HTTP bridge from an Omnia agent to
// Microsoft SharePoint via the Graph API. It is NOT a reusable/product
// connector. See docs/local-backlog/2026-06-02-sharepoint-adapter-plan.md.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := loadConfig()

	tokenSource, err := newAzureTokenSource(cfg)
	if err != nil {
		log.Error("create token source", "err", err.Error())
		os.Exit(1)
	}
	client := NewGraphClient(cfg.GraphBaseURL, cfg.SiteID, tokenSource, nil)

	mode := "serve"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	switch mode {
	case "seed":
		seeder := &Seeder{
			src:         client,
			memoryURL:   cfg.MemoryURL,
			workspaceID: cfg.WorkspaceID,
			http:        http.DefaultClient,
			log:         log,
		}
		n, err := seeder.Run(context.Background())
		if err != nil {
			log.Error("seed failed", "seeded", n, "err", err.Error())
			os.Exit(1)
		}
		log.Info("seed complete", "seeded", n)
	default:
		srv := NewServer(client, log)
		addr := ":" + cfg.Port
		log.Info("listening", "addr", addr)
		if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
			log.Error("server exited", "err", err.Error())
			os.Exit(1)
		}
	}
}

// newAzureTokenSource returns a Graph token source backed by an Entra app
// registration's client secret.
func newAzureTokenSource(cfg Config) (TokenSource, error) {
	cred, err := azidentity.NewClientSecretCredential(cfg.TenantID, cfg.ClientID, cfg.ClientSecret, nil)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) (string, error) {
		tok, err := cred.GetToken(ctx, policy.TokenRequestOptions{
			Scopes: []string{"https://graph.microsoft.com/.default"},
		})
		if err != nil {
			return "", err
		}
		return tok.Token, nil
	}, nil
}
