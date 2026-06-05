package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	wowhttp "wowdoc/internal/http"
)

func main() {
	if err := runWithArgs(os.Args[1:], http.ListenAndServe); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(listenAndServe func(string, http.Handler) error) error {
	return runWithArgs(nil, listenAndServe)
}

func runWithArgs(args []string, listenAndServe func(string, http.Handler) error) error {
	if len(args) > 0 && !(len(args) == 2 && args[0] == "mcp" && args[1] == "http") {
		return errors.New("unsupported command: " + strings.Join(args, " "))
	}
	cfg, err := wowhttp.LoadConfig(os.Getenv("WOWDOC_CONFIG"))
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	app := wowhttp.NewApp(cfg)
	defer app.Close()
	return listenAndServe(addr, app.Router())
}
