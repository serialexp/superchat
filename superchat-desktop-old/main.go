package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
)

//go:embed ../web-client/dist ../web-client/index.html
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Strip the "../web-client" prefix from embedded assets
	frontendAssets, err := fs.Sub(assets, "web-client")
	if err != nil {
		log.Fatal(err)
	}

	// Create application with options
	err = wails.Run(&options.App{
		Title:     "superchat-desktop",
		Width:     1024,
		Height:    768,
		Assets:    frontendAssets,
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err)
	}
}
