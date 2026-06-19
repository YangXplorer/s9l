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

// formDrivers are the drivers offered by the connection form's dropdown.
var formDrivers = []string{"sqlite", "postgres", "mysql", "sqlserver"}

// showConnForm opens the connection form overlay. When edit is nil it adds a new
// connection (n); otherwise it edits the given one (e), pre-filled. The password
// field is never pre-filled — leaving it blank on edit keeps the existing
// password reference. Save persists; Cancel / Esc closes without changes.
func (a *App) showConnForm(edit *config.ConnectionConfig) {
	init := config.ConnectionConfig{}
	if edit != nil {
		init = *edit
	}
	portStr := ""
	if init.Port != 0 {
		portStr = strconv.Itoa(init.Port)
	}

	form := tview.NewForm()
	form.AddInputField("ID", init.ID, 28, nil, nil).
		AddInputField("Name", init.Name, 28, nil, nil).
		AddDropDown("Driver", formDrivers, driverIndex(init.Driver), nil).
		AddInputField("Host", init.Host, 28, nil, nil).
		AddInputField("Port", portStr, 28, nil, nil).
		AddInputField("User", init.User, 28, nil, nil).
		AddInputField("Database", init.Database, 28, nil, nil).
		AddCheckbox("SSL", init.SSL, nil).
		AddPasswordField("Password", "", 28, '*', nil).
		AddInputField("Password ref", init.PasswordRef, 28, nil, nil)

	origID := ""
	title := " New connection — Esc: cancel "
	if edit != nil {
		origID = edit.ID
		title = " Edit connection — Esc: cancel "
	}
	form.AddButton("Save", func() {
		if err := a.submitConnForm(form, origID); err != nil {
			a.setError(err.Error())
			return
		}
		a.hideConnForm()
		if origID == "" {
			a.SetStatus("connection added")
		} else {
			a.SetStatus("connection updated")
		}
	})
	form.AddButton("Cancel", func() { a.hideConnForm() })
	form.SetFieldBackgroundColor(a.theme.Field).
		SetFieldTextColor(a.theme.FieldText).
		SetButtonBackgroundColor(a.theme.Field).
		SetButtonTextColor(a.theme.FieldText).
		SetLabelColor(a.theme.Title)
	form.SetBorder(true).SetTitle(title).SetTitleColor(a.theme.Title)
	form.SetBorderColor(a.theme.Focus)

	a.pages.AddPage("connform", centered(form, 52, 24), true, true)
	a.app.SetFocus(form)
	a.connFormOpen = true
}

// driverIndex returns the dropdown index of driver in formDrivers (0 if absent).
func driverIndex(driver string) int {
	for i, d := range formDrivers {
		if d == driver {
			return i
		}
	}
	return 0
}

func (a *App) hideConnForm() {
	a.pages.RemovePage("connform")
	a.connFormOpen = false
	a.app.SetFocus(a.navPanels()[a.focusIdx])
}

// submitConnForm reads the form into a ConnectionConfig and persists it: a new
// connection when origID is empty, otherwise an update of origID.
func (a *App) submitConnForm(form *tview.Form, origID string) error {
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
	if origID == "" {
		return a.saveConnection(cc, field("Password"))
	}
	return a.editConnection(origID, cc, field("Password"))
}

// validateConn checks the required fields shared by add and edit.
func validateConn(cc config.ConnectionConfig) error {
	if cc.ID == "" {
		return errors.New("id is required")
	}
	if cc.Driver == "" {
		return errors.New("driver is required")
	}
	if cc.Driver == "sqlite" && cc.Database == "" {
		return errors.New("sqlite requires a database (file path)")
	}
	return nil
}

// saveConnection validates and persists a new connection: it appends to the
// config (unique-id checked), stores any password in the secret store (the
// config keeps only a reference), writes config.yaml, and refreshes the list.
func (a *App) saveConnection(cc config.ConnectionConfig, password string) error {
	if err := validateConn(cc); err != nil {
		return err
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

// editConnection replaces the connection origID with cc. A non-empty password is
// stored in the secret store (config keeps only a ref); an empty password keeps
// the existing reference. On any failure the original entry is restored.
func (a *App) editConnection(origID string, cc config.ConnectionConfig, password string) error {
	if err := validateConn(cc); err != nil {
		return err
	}
	old, ok := a.cfg.Get(origID)
	if !ok {
		return fmt.Errorf("connection %q not found", origID)
	}
	// Charset has no form field; preserve the existing value so editing doesn't
	// silently drop it.
	cc.Charset = old.Charset
	if cc.ID != origID {
		if _, exists := a.cfg.Get(cc.ID); exists {
			return fmt.Errorf("connection %q already exists", cc.ID)
		}
	}
	if password != "" && cc.PasswordRef == "" {
		cc.PasswordRef = secret.KeychainRef(cc.ID)
	}

	a.cfg.Remove(origID)
	if err := a.cfg.Add(cc); err != nil {
		_ = a.cfg.Add(old) // restore
		return err
	}
	if password != "" {
		if err := a.store.Set(secret.Service, secret.ConnPasswordKey(cc.ID), password); err != nil {
			a.cfg.Remove(cc.ID)
			_ = a.cfg.Add(old)
			return fmt.Errorf("store password: %w", err)
		}
	}
	if err := a.cfg.Save(); err != nil {
		a.cfg.Remove(cc.ID)
		_ = a.cfg.Add(old)
		return err
	}
	a.populateConnections()
	return nil
}

// deleteConnection removes a connection and its keychain password (best-effort).
func (a *App) deleteConnection(id string) error {
	if !a.cfg.Remove(id) {
		return fmt.Errorf("connection %q not found", id)
	}
	if err := a.cfg.Save(); err != nil {
		return err
	}
	_ = a.store.Delete(secret.Service, secret.ConnPasswordKey(id)) // best-effort
	a.populateConnections()
	return nil
}

// selectedConn returns the connection highlighted in the Connections tree. The
// current node is either a connection node or one of its database children, so
// resolve a database node up to its owning connection.
func (a *App) selectedConn() (config.ConnectionConfig, bool) {
	node := a.connTree.GetCurrentNode()
	if node == nil {
		return config.ConnectionConfig{}, false
	}
	switch ref := node.GetReference().(type) {
	case connNodeRef:
		return ref.cc, true
	case dbNodeRef:
		if cc, ok := a.cfg.Get(ref.connID); ok {
			return cc, true
		}
	}
	return config.ConnectionConfig{}, false
}

// editSelectedConn opens the form pre-filled with the highlighted connection.
func (a *App) editSelectedConn() {
	if cc, ok := a.selectedConn(); ok {
		a.showConnForm(&cc)
	}
}

// confirmDeleteSelectedConn asks for confirmation, then deletes the highlighted
// connection.
func (a *App) confirmDeleteSelectedConn() {
	cc, ok := a.selectedConn()
	if !ok {
		return
	}
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Delete connection %q?", cc.ID)).
		AddButtons([]string{"Cancel", "Delete"}).
		SetDoneFunc(func(_ int, label string) {
			a.pages.RemovePage("confirmdel")
			a.confirmOpen = false
			a.app.SetFocus(a.navPanels()[a.focusIdx])
			if label == "Delete" {
				if err := a.deleteConnection(cc.ID); err != nil {
					a.setError(err.Error())
					return
				}
				a.SetStatus("connection deleted")
			}
		})
	modal.SetBackgroundColor(a.theme.Field).SetTextColor(a.theme.FieldText)
	a.pages.AddPage("confirmdel", modal, true, true)
	a.app.SetFocus(modal)
	a.confirmOpen = true
}
