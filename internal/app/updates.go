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

const (
	// updateEvery is how often the app looks for a newer version while it
	// stays open, for the hint the account row shows.
	updateEvery = time.Hour
	// updateLeast is the least time between the looks the window asks for
	// as it shows again, so that coming back to it asks the feed at most
	// once in it.
	updateLeast = 10 * time.Minute
)

// useUpdater checks GitHub's releases for a new version soon after launch
// and then every hour, and offers it in an update window of native UI, in
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

// checkUpdates looks for a newer version now, and again every updateEvery,
// for the hint the account row shows while there is one.
func (a *app) checkUpdates() {
	a.checkedAt = time.Now()
	a.fetchUpdate()
	time.AfterFunc(updateEvery, func() {
		mygo.RunOnMain(func() {
			if a.win != nil {
				a.checkUpdates()
			}
		})
	})
}

// checkUpdatesSoon looks again as the window shows, unless an update is
// already known, or the feed was asked within updateLeast: an app left open
// hears of a version sooner than the next updateEvery.
func (a *app) checkUpdatesSoon() {
	if !updateDue(a.checkedAt, a.update != nil, time.Now()) {
		return
	}
	a.checkedAt = time.Now()
	a.fetchUpdate()
}

// updateDue reports whether a look the window asks for should ask the feed:
// when none is known, and not more often than updateLeast.
func updateDue(checkedAt time.Time, known bool, now time.Time) bool {
	return !known && (checkedAt.IsZero() || now.Sub(checkedAt) >= updateLeast)
}

// fetchUpdate asks the feed in the background, and keeps what it offers.
func (a *app) fetchUpdate() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		up, err := checkUpdate(ctx)
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
