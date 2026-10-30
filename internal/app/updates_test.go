package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Small dialogs close on outside clicks; settings stay centered when their heading is dragged.
func TestSmallDialogsDismiss(t *testing.T) {
	for _, name := range []string{"settings", "login", "collect", "name", "delete"} {
		t.Run(name, func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			switch name {
			case "settings":
				a.settingsOpen = true
			case "login":
				a.login.open = true
			case "collect":
				a.collecting = collectDialog{open: true, tr: &netease.Track{ID: 1}}
			case "name":
				a.naming.open = true
			case "delete":
				a.deleting.open = true
			}
			tt := ui.NewTester(a.view, 1200, 800)
			tt.Frame()
			tt.Advance(time.Second)
			tt.Frame()
			if name == "settings" {
				r, ok := tt.Find("设置")
				if !ok || r.X+r.W/2 != 600 || r.Y+r.H/2 != 400 {
					t.Fatalf("settings bounds = %+v", r)
				}
				tt.Press(r.X+60, r.Y+28)
				tt.Move(r.X+180, r.Y+100)
				tt.Release(r.X+180, r.Y+100)
				tt.Frame()
				moved, _ := tt.Find("设置")
				if moved != r {
					t.Fatalf("settings moved from %+v to %+v", r, moved)
				}
			}
			tt.ClickAt(20, 300)
			tt.Frame()
			if a.settingsOpen || a.login.open || a.collecting.open || a.naming.open || a.deleting.open {
				t.Fatal("outside click left the dialog open")
			}
		})
	}
}

// fakeUpdater swaps the update plugin's preferences for ones in memory,
// counting the checks asked for, until the returned function puts them
// back.
func fakeUpdater(checks, downloads *bool, checked *int) func() {
	check, getChecks, setChecks := checkForUpdates, updateChecks, setUpdateChecks
	getDownloads, setDownloads, last := updateDownloads, setUpdateDownloads, lastUpdateCheck
	checkForUpdates = func() { *checked++ }
	updateChecks = func() bool { return *checks }
	setUpdateChecks = func(on bool) { *checks = on }
	updateDownloads = func() bool { return *downloads }
	setUpdateDownloads = func(on bool) { *downloads = on }
	lastUpdateCheck = func() time.Time { return time.Time{} }
	return func() {
		checkForUpdates, updateChecks, setUpdateChecks = check, getChecks, setChecks
		updateDownloads, setUpdateDownloads, lastUpdateCheck = getDownloads, setDownloads, last
	}
}

// Turning off checking for updates turns it off in the updater, and
// installing updates without asking can't be turned on without it.
func TestTurnOffUpdateChecks(t *testing.T) {
	checks, downloads, checked := true, false, 0
	defer fakeUpdater(&checks, &downloads, &checked)()
	a := newTestApp()
	a.settings = defaultSettings()
	a.updates = true
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	// The row's title is labelled as its switch is: click the switch, at
	// the row's end.
	toggle := func(title string) {
		t.Helper()
		top, ok := tt.Find(title)
		if !ok {
			t.Fatalf("no row %s", title)
		}
		tt.ClickAt(560-24-20-22.5, top.Y+top.H/2)
		tt.Frame()
	}
	toggle("自动检查更新")
	if checks {
		t.Fatal("checks for updates still")
	}
	toggle("自动下载并安装")
	if downloads {
		t.Fatal("turned on installing updates without checking for them")
	}
	// It does turn on while the updater checks.
	checks = true
	tt.Frame()
	toggle("自动下载并安装")
	if !downloads {
		t.Fatal("didn't turn on installing updates")
	}
}

// 检查更新 in the settings checks for updates as the user asked.
func TestCheckForUpdatesFromSettings(t *testing.T) {
	checks, downloads, checked := true, false, 0
	defer fakeUpdater(&checks, &downloads, &checked)()
	a := newTestApp()
	a.settings = defaultSettings()
	a.updates = true
	a.profile = &netease.Profile{UserID: 1}
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if err := tt.Click("检查更新"); err != nil {
		t.Fatal(err)
	}
	if checked != 1 {
		t.Fatalf("checked %d times", checked)
	}
}

// The data kept under the app's old name moves to the directory of its
// name, which Path made empty, but not over data kept there already.
func TestMoveOldData(t *testing.T) {
	root := t.TempDir()
	old, dir := filepath.Join(root, oldName), filepath.Join(root, "MyGo 云音乐")
	os.MkdirAll(old, 0o700)
	os.WriteFile(filepath.Join(old, "settings.json"), []byte("old"), 0o600)
	os.MkdirAll(dir, 0o700)
	moveDir(old, dir)
	if b, _ := os.ReadFile(filepath.Join(dir, "settings.json")); string(b) != "old" {
		t.Fatalf("settings %q after moving", b)
	}

	os.MkdirAll(old, 0o700)
	os.WriteFile(filepath.Join(old, "settings.json"), []byte("older"), 0o600)
	moveDir(old, dir)
	if b, _ := os.ReadFile(filepath.Join(dir, "settings.json")); string(b) != "old" {
		t.Fatalf("settings %q, moved over what was there", b)
	}
}
