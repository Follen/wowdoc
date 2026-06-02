package main

import (
	"fmt"
	"net/http"

	wowhttp "wowdoc/internal/http"
)

func main() {
	cfg := wowhttp.DefaultConfig()
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	_ = http.ListenAndServe(addr, wowhttp.NewApp(cfg).Router())
}
