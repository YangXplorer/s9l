package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/secret"

	"github.com/rivo/tview"
)

// formDrivers are the drivers offered by the new-connection form's dropdown.
var formDrivers = []string{"sqlite", "postgres", "mysql", "sqlserver"}

// showConnForm opens the "add connection" form overlay (n). Save persists the
// connection; Cancel / Esc closes without changes.
func (a *App) showConnForm() {
	form := tview.NewForm()
	form.AddInputField("ID", "", 28, nil, nil).
		AddInputField("Name", "", 28, nil, nil).
		AddDropDown("Driver", formDrivers, 0, nil).
		AddInputField("Host", "", 28, nil, nil).
		AddInputField("Port", "", 28, nil, nil).
		AddInputField("User", "", 28, nil, nil).
		AddInputField("Database", "", 28, nil, nil).
		AddCheckbox("SSL", false, nil).
		AddPasswordField("Password", "", 28, '*', nil).
		AddInputField("Password ref", "", 28, nil, nil)
	form.AddButton("Save", func() {
		if err := a.submitConnForm(form); err != nil {
			a.setError(err.Error())
			return
		}
		a.hideConnForm()
		a.SetStatus("connection added")
	})
	form.AddButton("Cancel", func() { a.hideConnForm() })
	form.SetBorder(true).SetTitle(" New connection — Esc: cancel ").SetTitleColor(a.theme.Title)
	form.SetBorderColor(a.theme.Focus)

	a.pages.AddPage("connform", centered(form, 52, 24), true, true)
	a.app.SetFocus(form)
	a.connFormOpen = true
}

func (a *App) hideConnForm() {
	a.pages.RemovePage("connform")
	a.connFormOpen = false
	a.app.SetFocus(a.navPanels()[a.focusIdx])
}

// submitConnForm reads the form into a ConnectionConfig and persists it.
func (a *App) submitConnForm(form *tview.Form) error {
	field := func(label string) string {
		if it, ok := form.GetFormItemByLabel(label).(*tview.InputField); ok {
			return strings.TrimSpace(it.GetText())
		}
		return ""
	}
	_, driver := form.GetFormItemByLabel("Driver").(*tview.DropDown).GetCurrentOption()
	ssl := form.GetFormItemByLabel("SSL").(*tview.Checkbox).IsChecked()

	port := 0
	if p := field("Port"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			return fmt.Errorf("port: must be a number")
		}
		port = n
	}
	cc := config.ConnectionConfig{
		ID:          field("ID"),
		Name:        field("Name"),
		Driver:      driver,
		Host:        field("Host"),
		Port:        port,
		User:        field("User"),
		Database:    field("Database"),
		SSL:         ssl,
		PasswordRef: field("Password ref"),
	}
	return a.saveConnection(cc, field("Password"))
}

// saveConnection validates and persists a new connection: it appends to the
// config (unique-id checked), stores any password in the secret store (the
// config keeps only a reference), writes config.yaml, and refreshes the list.
func (a *App) saveConnection(cc config.ConnectionConfig, password string) error {
	if cc.ID == "" {
		return errors.New("id is required")
	}
	if cc.Driver == "" {
		return errors.New("driver is required")
	}
	if cc.Driver == "sqlite" && cc.Database == "" {
		return errors.New("sqlite requires a database (file path)")
	}
	// A typed password is stored in the secret store; the config references it.
	if password != "" && cc.PasswordRef == "" {
		cc.PasswordRef = secret.KeychainRef(cc.ID)
	}
	if err := a.cfg.Add(cc); err != nil { // validates the id is unique
		return err
	}
	if password != "" {
		if err := a.store.Set(secret.Service, secret.ConnPasswordKey(cc.ID), password); err != nil {
			a.cfg.Remove(cc.ID) // roll back so a retry isn't blocked by a dup id
			return fmt.Errorf("store password: %w", err)
		}
	}
	if err := a.cfg.Save(); err != nil {
		a.cfg.Remove(cc.ID)
		if password != "" {
			_ = a.store.Delete(secret.Service, secret.ConnPasswordKey(cc.ID))
		}
		return err
	}
	a.populateConnections()
	return nil
}
