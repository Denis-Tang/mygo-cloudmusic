package app

import (
	"context"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
)

// The update plugin's check and preferences, which tests swap: they don't
// use the plugin, whose functions panic without it.
var (
	checkForUpdates    = updater.CheckForUpdates
	updateChecks       = updater.AutomaticChecks
	setUpdateChecks    = updater.SetAutomaticChecks
	updateDownloads    = updater.AutomaticDownloads
	setUpdateDownloads = updater.SetAutomaticDownloads
	lastUpdateCheck    = updater.LastCheck
	// checkUpdate asks the feed for a version newer than this one, which
	// tests replace. A build that cannot update itself, as a development
	// one, has none.
	checkUpdate = func(ctx context.Context) (*mygo.Update, error) {
		if !mygo.Updater.Enabled() {
			return nil, nil
		}
		return mygo.Updater.Check(ctx)
	}
)

// updateEvery is how often the app looks for a newer version, for the hint
// the account row shows.
const updateEvery = 12 * time.Hour

// useUpdater checks GitHub's releases for a new version soon after launch
// and then once a day, and offers it in an update window of native UI, in
// Chinese. It must run before App.Run.
func (a *app) useUpdater() {
	mygo.Use(native.New(updater.Options{Language: "zh-Hans"}))
	a.updates = true
	// A check or an answer in the update window changes what the settings
	// show.
	updater.OnChange(func() {
		a.redraw()
	})
}

// checkUpdates looks for a newer version at launch, and then every
// updateEvery, for the hint the account row shows while there is one.
func (a *app) checkUpdates() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		up, err := checkUpdate(ctx)
		time.AfterFunc(updateEvery, func() {
			mygo.RunOnMain(func() {
				if a.win != nil {
					a.checkUpdates()
				}
			})
		})
		if err != nil {
			return
		}
		a.post(func() {
			a.update = up
		})
	}()
}

// updateHint is the version of an update the account row hints at, or ""
// when there is none, or when the listener has seen the one there is.
func (a *app) updateHint() string {
	if a.update == nil || a.update.Version == a.settings.UpdateSeen {
		return ""
	}
	return a.update.Version
}
