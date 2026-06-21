// Command fivepoint is the Wails desktop entry point. It embeds the static
// frontend and exposes the app service (vault + wallet operations) to the
// webview as window.go.app.App.*.
package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/rhymeswithlimo/fivepoint/internal/app"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var assetsFS embed.FS

func main() {
	assets, err := fs.Sub(assetsFS, "frontend")
	if err != nil {
		log.Fatalf("embed frontend: %v", err)
	}

	a := app.New()

	err = wails.Run(&options.App{
		Title:            "Fivepoint",
		Width:            480,
		Height:           860,
		MinWidth:         360,
		MinHeight:        600,
		BackgroundColour: &options.RGBA{R: 5, G: 5, B: 5, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        a.Startup,
		Bind:             []interface{}{a},
	})
	if err != nil {
		log.Fatalf("fivepoint: %v", err)
	}
}
