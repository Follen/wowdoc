package main

import (
	"fmt"
	"net/http"
	"os"

	wowhttp "wowdoc/internal/http"
)

func main() {
	if err := run(http.ListenAndServe); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(listenAndServe func(string, http.Handler) error) error {
	cfg := wowhttp.DefaultConfig()
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return listenAndServe(addr, wowhttp.NewApp(cfg).Router())
}
