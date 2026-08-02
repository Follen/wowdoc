package main

import (
	"os"

	"github.com/follenfang/wowdoc/internal/app"
)

func main() {
	os.Exit(app.RunWowdata(os.Args[1:], os.Stdout, os.Stderr))
}
