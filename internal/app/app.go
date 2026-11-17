package app

import (
	"log"
	"os"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/control"
	"netease-music-mygo/internal/diag"
	"netease-music-mygo/internal/local"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

type pageKind int

const (
	pageHome pageKind = iota
	pageDaily
	pageCharts
	pagePlaylist
	pageSearch
	pageFM
	pageArtist
	pageAlbum
	pageCollected
	pageUser
	pagePodcast
	pagePodcasts
	pageLocal
	pageBili
	pageBiliPlaylist
)

type route struct {
	page pageKind
	// id is the playlist, artist, album, user or podcast the page shows.
	id int64
	// query and kind are what a search page searched for.
	query string
	kind  netease.SearchType
}

// app is the state both windows show. Everything in it is read and changed
// on the main thread: by the views, and by goroutines through post.
type app struct {
	win   *mygo.Window
	notch *notch
	tray  *mygo.Tray // the icon in the menu bar or notification area, while it shows
	art   *artCache
	pl    *player.Player

	// lib is the music of the listener's own folders, and locals its
	// tracks as the pages list them: see ui_local.go.
	lib           *local.Library
	libReady      bool
	localScanning bool
	locals        []netease.Track

	// biliTracks are the videos searched for or ranked, as tracks, and
	// biliRemotes what plays each: see ui_bili.go.
	biliTracks   []netease.Track
	biliHits     []bilibili.Video
	biliFilter   int64 // the uploader whose playlists the page lists
	biliBuilding map[string]bool
	biliVideos   map[int64]bilibili.Video
	biliRemotes  map[int64]player.Remote
	// biliLocals are the video's stretches kept on this machine, which play
	// from their file.
	biliLocals map[int64]player.Local
	// biliSliceAsk asks whether to cut a playlist's stretches into files.
	biliSliceAsk biliSliceAsk
	plAdding     playlistAdding
	biliNewPL    biliNewPlaylist
	// localAdds are songs of this machine in the site's playlists, kept
	// here alone: see local_adds.go.
	localAdds   map[int64][]biliItem
	biliQuery   string
	biliBox     ui.Handle // the search field of the page, for its focus look
	biliShown   string    // the query whose results the page lists
	biliSources map[int64]biliSource
	biliLoading bool
	biliErr     string

	// The uploaders the listener follows, kept on this machine, and what
	// they posted: see bili_follows.go.
	follows         []biliFollow
	biliUsers       []bilibili.User
	biliFeed        []bilibili.Video
	biliFeedTr      []netease.Track
	biliNew         map[string]bool
	biliNewNum      int
	biliFeedErr     string
	biliLoadingFeed bool
	biliPoll        *time.Timer
	biliPulled      time.Time

	// control serves the API tests drive the app with, when MYGO_CONTROL
	// is set: see control.go.
	control       *control.Server
	biliPlaylists []biliPlaylist
	// biliPreview is the playlist the video last opened made: it stays in
	// memory, out of the sidebar and what the listener keeps, until it is
	// collected. A new search drops it.
	biliPreview     *biliPlaylist
	biliNaming      biliNaming
	biliDeleting    biliDeleting
	biliPLCollapsed bool
	biliDraft       biliDraft
	// crashed is what panicked the view, shown in place of it until the
	// app starts again.
	crashed string
	pal     palette

	frameDark bool // the appearance the window's frame was drawn in

	taskbar taskbarControls // the taskbar button's thumbnail toolbar (Windows)

	route   route
	history []route
	forward []route // the pages gone back from, the last first to go to

	profile      *netease.Profile
	mine, saved  []netease.Playlist // the listener's playlists: made, and saved
	recPlaylists []netease.Playlist // the day's playlists picked for the listener
	login        loginState
	accountOpen  bool           // the dropdown of the account row shows
	naming       nameDialog     // the name of a playlist asked for
	deleting     deleteDialog   // whether to delete a playlist, asked
	collecting   collectDialog  // the playlists a song of the playing page may go into
	biliCollect  biliCollecting // the name a Bilibili playlist takes on the site
	biliAcc      *biliAccount   // the Bilibili account signed in, read alone
	biliLogin    biliLogin
	biliImport   biliImport
	collectedTab int            // which kind of what the listener keeps 我的收藏 lists
	liked        map[int64]bool // the songs the listener likes
	toast        string         // a message to show once
	picked       pickedUp       // the song a login picked up, waiting to play

	// The episodes the listener likes, and doesn't, as far as NetEase has
	// answered, and those asked about: see ui_like.go.
	likedPrograms, askedPrograms map[int64]bool

	daily     []netease.Track
	dailySpin spinState // the record on the daily card
	dailyBars barState  // the bars on its play button
	// The album publish times kept, and whether they are read and new:
	// see albumtime.go. They are for the main thread alone.
	pubLoaded   bool
	pubTimes    map[int64]int64
	pubAsked    map[int64]bool
	pubDirty    bool
	featured    []netease.Playlist
	charts      []netease.Playlist
	homeErr     string
	homeLoading bool
	homeAt      time.Time // when what the home page shows was asked for

	playlists map[int64]*netease.Playlist
	loadingPl map[int64]bool
	plErr     map[int64]string
	// The albums the listener collects: asked for once the page opens,
	// and kept, for the 收藏 album's page shows them again.
	collected        []netease.AlbumProfile
	collectedLoaded  bool
	collectedLoading bool
	collectedErr     string
	artists          map[int64]*artistData
	artistTab        int // what an artist's page lists: artistHot, artistLatest, artistAlbums or artistEvents
	albums           map[int64]*albumData
	users            map[int64]*userData
	podcasts         map[int64]*podcastData
	podcastTab       *podcastTab // the podcasts tab's lists
	trackList        ui.ListState
	// Where the page and the sidebar are scrolled, kept while Now Playing
	// covers them, unbuilt.
	pageScroll ui.ScrollState
	navScroll  ui.ScrollState
	scrolled   route // the page trackList and pageScroll are of

	mineCollapsed     bool
	mineListCollapsed bool
	savedCollapsed    bool
	// mineSrc is which playlists the sidebar's 我的歌单 lists: the site's
	// or the ones made here.
	mineSrc int

	query       string
	searchBox   ui.Handle
	searches    map[route]*searchData // the results of each search, by its page
	searched    route                 // the search last shown, which the sidebar goes back to
	suggestions suggestState          // what the search field's query may be after

	nowPlaying    bool
	npOpen        bool      // Now Playing as it last showed: open or not
	npSettled     bool      // Now Playing at rest, open or closed
	npHeroFromBar bool      // the open cover grows from the bar's, not from a turn already under way
	npChangedAt   time.Time // when it last opened or closed
	npPane        int       // paneLyrics, paneQueue, paneStage, paneComments, paneProgram or paneCaptions
	npPaneTop     float32   // how far the cover beside the pane starts below its top
	npTabsH       float32   // how tall the pane's tabs were, last laid out
	lyricScroll   ui.ScrollState
	lyricGlide    lyricGlide
	lyricAt       int
	lyricTrack    int64
	queueList     ui.ListState
	queueAt       int // the row of the queue pane last scrolled to, -1 while it is hidden
	comments      commentState
	tabCount      rollState // the count on the comments' tab, as it rolls
	npDrawer      npDrawer  // the bar's [...] panel, of the queue and the comments

	fmMode        int    // the FM mode chosen, in fmModes
	radioStarting string // the radio asked for its first songs, or ""
	pendingPlay   route  // the last source asked to play once loaded
	pendingTrack  *netease.Track

	scrub      float64
	scrubbing  bool
	volume     float64
	volumeOpen bool // the volume popover of the player bar shows
	mutedFrom  float64

	rowBars barState
	// drag, dragMove and dragGen are the sidebar row a group drags, where it
	// would land, and the drops so far: see ui_dragrow.go.
	drag        *dragCard
	dragMove    *dragMove
	dragGen     int
	dragDrops   []func()
	dragSettled bool
	// underTop says whether content has scrolled under the top bar.
	underTop      bool
	barScrub      scrubState
	npScrub       scrubState
	backdropCache backdropCache // the backdrop of Now Playing, as last painted
	npArt         *artwork      // the cover whose colors Now Playing shows
	npCoverState  coverState    // the cover of Now Playing, as it is tilted and flicked
	quiet         quietState    // the quiet parts of the song playing, for its hot comments
	captions      captionState  // the episode playing's captions, and the model that writes them
	sig           playerSig
	quitting      bool

	settings     settings
	settingsOpen bool
	// The immersive page of Now Playing keeps states of its own, where the
	// panes keep to theirs: the bar's scrubber, the lyrics the picture's
	// click asks for, and the lyrics' scroll, glide and line. See
	// ui_immersive.go.
	coverScrub scrubState
	npActiveAt time.Time    // when the listener last acted on the playing page
	cacheUsed  int64        // the bytes the song cache takes, -1 until measured
	updates    bool         // the update plugin is used (useUpdater)
	update     *mygo.Update // the newer version the feed offers, or nil
	checkedAt  time.Time    // when the feed was last asked, for checkUpdatesSoon

	restored     bool   // playback picked up where it was left
	savedSession string // the session as last kept
	sessionDue   bool   // a save of the session is due
}

// post runs fn on the main thread and redraws both windows. Without a
// window, as in tests, it drops fn.
func (a *app) post(fn func()) {
	if a.win == nil {
		return
	}
	a.win.Update(func() {
		defer diag.Recover("post")
		fn()
		a.redraw()
	})
}

func (a *app) redraw() {
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.notch != nil && a.notch.win != nil {
		a.notch.win.Invalidate()
	}
}

// shotHook, when set, drives the app to check its design.
var shotHook func(a *app)

func (a *app) showMain() {
	if a.win != nil {
		if a.win.IsMinimized() {
			a.win.Restore()
		}
		a.win.Show()
		a.win.Focus()
		// From the menu bar alone the app may be inactive.
		mygo.App.Focus()
	}
}

// Run runs the app until it quits.
func Run() error {
	// One instance: a second launch hands its command line to the first,
	// which shows its window (OnSecondInstance), and exits.
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	a := &app{
		playlists:   map[int64]*netease.Playlist{},
		loadingPl:   map[int64]bool{},
		plErr:       map[int64]string{},
		artists:     map[int64]*artistData{},
		albums:      map[int64]*albumData{},
		users:       map[int64]*userData{},
		podcasts:    map[int64]*podcastData{},
		podcastTab:  &podcastTab{},
		searches:    map[route]*searchData{},
		lyricAt:     -1,
		queueAt:     -1,
		cacheUsed:   -1,
		biliVideos:  map[int64]bilibili.Video{},
		biliRemotes: map[int64]player.Remote{},
		biliLocals:  map[int64]player.Local{},
		pubAsked:    map[int64]bool{},
	}
	moveOldData()
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
		if _, err := diag.Setup(dir); err != nil {
			log.Printf("diag: %v", err)
		}
	}
	diag.Logf("version %s", mygo.App.Version())
	a.useUpdater()
	mygo.App.WhenReady(func() {
		a.settings = loadSettings()
		a.settings.applyAppearance()
		a.npPane = a.lyricsPaneChosen()
		// Before the window: on Windows it lays out its content as it is
		// made, and the view asks the player what plays.
		a.art = newArtCache(a.post)
		a.pl = player.New(a.post)
		a.pl.Quality = func() string { return a.settings.Quality }
		a.pl.Normalize = func() bool { return a.settings.Normalize }
		a.pl.Cache = newSongCache(a.settings.CacheMB)
		a.pl.Blocked = a.blocked
		a.pl.RemoteOf = func(id int64) (player.Remote, bool) {
			r, ok := a.biliRemotes[id]
			return r, ok
		}
		a.pl.LocalOf = func(id int64) (player.Local, bool) {
			if loc, ok := a.biliLocals[id]; ok {
				return loc, true
			}
			if a.lib == nil || id >= 0 {
				return player.Local{}, false
			}
			t, ok := a.lib.Track(id)
			if !ok {
				return player.Local{}, false
			}
			return player.Local{File: t.Path, Cover: t.Cover, LRC: t.LRC}, true
		}
		a.pl.Changed = func() {
			a.keepSession()
			a.syncCaptions()
		}
		a.volume = a.settings.Volume
		a.pl.SetVolume(a.volume)
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:         mygo.App.Name(),
			Width:         1180,
			Height:        780,
			MinWidth:      880,
			MinHeight:     580,
			StateKey:      "main",
			TitleBarStyle: mygo.TitleBarHidden,
			// Windows' caption buttons are drawn in the app, as the official
			// client draws them; other systems keep their native controls.
			HideWindowButtons: runtime.GOOS == "windows",
			// As tall as the edge over a page (Linux, Windows).
			TitleBarHeight: buttonsHeight,
			// Inset from the corner, within the sidebar, as macOS 26's
			// apps with a sidebar place them.
			TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
			Vibrancy:             mygo.VibrancySidebar,
			BackgroundColor:      "light-dark(#ffffff, #141416)",
			Content:              ui.View(a.view),
			// Nearly clear until its first frame: opened straight, the
			// window showed its background while the view laid out.
			Opacity: 0.01,
		})
		a.win.OnReadyToShow(func() {
			a.win.SetOpacity(1)
			a.win.Focus()
		})
		if os.Getenv("MYGO_CONTROL") != "" {
			a.startControl()
		}
		if os.Getenv("MYGO_HEADLESS") != "" {
			// Driven alone: no window over the screen, and no focus taken.
			a.win.SetOpacity(0)
			a.win.Hide()
		}
		a.taskbar.init(a.win.NativeHandle(), a)
		a.setMenu()
		a.boot()
		// The notch player is macOS's, over its menu bar: elsewhere it would
		// hang at the top of the screen as a black box.
		if runtime.GOOS == "darwin" {
			a.notch = newNotch(a)
		}
		// From the menu bar alone, the Dock icon shows while the window
		// does.
		a.syncDock()
		a.win.OnShow(func() {
			a.syncDock()
			a.taskbar.refresh()
			// Shown again, the window can show Explorer's backdrop before
			// the first frame of its content: nearly clear until it draws.
			a.win.SetOpacity(0.01)
			a.win.Invalidate()
			time.AfterFunc(150*time.Millisecond, func() { a.win.SetOpacity(1) })
			a.checkUpdatesSoon()
		})
		a.win.OnHide(a.syncDock)
		// Shown or switched to the morning after, it shows the new day's.
		a.win.OnFocus(func() {
			a.freshenHome()
			a.checkUpdatesSoon()
		})
		a.applyTray()
		if shotHook != nil {
			shotHook(a)
		}
		// Closing the window hides it: music plays on, and the Dock icon
		// or the tray's brings it back. A Linux desktop without a tray
		// has neither, so there closing it quits.
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if a.quitting {
				return
			}
			e.PreventDefault()
			if runtime.GOOS == "darwin" || a.tray != nil {
				a.win.Hide()
			} else {
				mygo.App.Quit()
			}
		})
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		a.settings.Volume = a.volume
		a.settings.save()
		a.saveSession()
		netease.KeepSession()
		if write := a.transcriptWrite(); write != nil {
			write()
		}
	})
	// Windows leaves an icon in the notification area after its app until
	// the pointer passes over it.
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		a.taskbar.close()
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(bool) { a.showMain() })
	// Started again, from the menu, the tray or a shortcut, while this one
	// runs: show the window it has, even when only the tray is left.
	mygo.App.OnSecondInstance(func([]string, string) { a.showMain() })
	return mygo.App.Run()
}
