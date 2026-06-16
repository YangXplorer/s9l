package tui_test

import (
	"testing"
	"time"

	"github.com/YangXplorer/s9l/internal/tui"

	"github.com/gdamore/tcell/v2"
)

// TestAppQuitsOnQ drives the scaffold with a SimulationScreen: after the first
// draw it sends 'q' and the app must exit cleanly (no real terminal needed).
func TestAppQuitsOnQ(t *testing.T) {
	a := tui.New(tui.Options{})
	a.SetScreen(tcell.NewSimulationScreen(""))
	a.OnReady(func() { a.SendKey('q') })

	done := make(chan error, 1)
	go func() { done <- a.Run() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		a.Stop()
		t.Fatal("TUI did not quit on 'q' within 10s")
	}
}
