package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/widget/material"

	"github.com/aeolun/superchat/cmd/client-gui/ui"
	"github.com/aeolun/superchat/pkg/client"
)

var Version = "dev"

func main() {
	// Parse command-line flags
	throttle := flag.Int("throttle", 0, "Throttle bandwidth (bytes/sec, e.g. 600 for 14.4k modem)")
	flag.Parse()
	// Determine state path (same logic as terminal client)
	xdgData := os.Getenv("XDG_DATA_HOME")
	if xdgData == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to get home directory: %v", err)
		}
		xdgData = filepath.Join(homeDir, ".local", "share")
	}
	statePath := filepath.Join(xdgData, "superchat", "state.db")

	// Open state database
	state, err := client.OpenState(statePath)
	if err != nil {
		log.Fatalf("Failed to open state database: %v", err)
	}
	defer state.Close()

	// Create connection to server
	serverAddr := "superchat.win:6465"
	conn, err := client.NewConnection(serverAddr)
	if err != nil {
		log.Fatalf("Failed to create connection: %v", err)
	}
	defer conn.Close()

	// Apply throttle if specified
	if *throttle > 0 {
		conn.SetThrottle(*throttle)
		fmt.Printf("Throttling bandwidth to %d bytes/sec\n", *throttle)
	}

	// Connect to server
	if err := conn.Connect(); err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}

	fmt.Printf("Connected to %s\n", serverAddr)

	// Start GUI in goroutine
	go func() {
		// Create window
		w := new(app.Window)
		w.Option(
			app.Title("SuperChat"),
			app.Size(1024, 768),
		)

		// Note: Window icon must be set via platform-specific methods:
		// - macOS: Set in app bundle Info.plist (requires .app bundle)
		// - Windows: Embed .ico in executable resources (requires build script)
		// - Linux: Set via .desktop file or X11 properties
		// The icon from pkg/client/assets/icon.png can be used for these purposes.

		// Create theme
		th := material.NewTheme()

		// Create UI state (pass window for invalidation)
		appUI := ui.NewApp(conn, state, th, Version, *throttle, w)

		// Event loop
		var ops op.Ops
		for {
			switch e := w.Event().(type) {
			case app.DestroyEvent:
				os.Exit(0)
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)

				// Layout handles keyboard events internally via handleKeyboardShortcuts()
				appUI.Layout(gtx)
				e.Frame(gtx.Ops)
			}
			// Request redraw to process any state changes
			w.Invalidate()
		}
	}()

	app.Main()
}
