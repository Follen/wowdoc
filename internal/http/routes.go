package http

import (
	"encoding/json"
	"net/http"
)

type App struct {
	cfg   Config
	pools *Pools
}

func NewApp(cfg Config) *App {
	return &App{cfg: cfg, pools: NewPools(cfg.Contexts.MaxSourceContexts, cfg.Contexts.MaxIndexContexts)}
}

func (a *App) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/help", a.help)
	mux.HandleFunc("/mcp", a.mcp)
	return mux
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"sources":            []any{},
		"clients":            []any{},
		"invalidDirectories": []any{},
		"pools":              a.pools.Stats(),
		"recentErrors":       []any{},
	})
}

func (a *App) help(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]string{"mcp": "/mcp", "health": "/health"})
}

func (a *App) mcp(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
