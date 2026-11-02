package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"clashtraffic/internal/clash"
	"clashtraffic/internal/collector"
	"clashtraffic/internal/i18n"
	"clashtraffic/internal/mesh"
	"clashtraffic/internal/sessions"
	"clashtraffic/internal/store"
	"clashtraffic/internal/syncer"
)

// ClashStatus tells whether the app reaches the Clash core.
type ClashStatus struct {
	Connected bool   `json:"connected"`
	Endpoint  string `json:"endpoint"`
	Source    string `json:"source"` // where the endpoint was found
	Version   string `json:"version"`
	Error     string `json:"error"`
	// FindProcessMode of the core: "always" finds the process of every
	// connection, "strict" of few, "off" of none.
	FindProcessMode string `json:"findProcessMode"`
	Since           int64  `json:"since"` // unix ms of the connection
}

// SyncStatus tells how syncing with the other devices goes.
type SyncStatus struct {
	Kind       SyncKind `json:"kind"`
	Running    bool     `json:"running"`
	LastSync   int64    `json:"lastSync"` // unix ms of the last sync that finished
	LastError  string   `json:"lastError"`
	Uploaded   int      `json:"uploaded"`
	Downloaded int      `json:"downloaded"`
	Devices    int      `json:"devices"` // other devices on the remote
}

// RatePoint is the speed at a moment.
type RatePoint struct {
	T    int64 `json:"t"` // unix ms
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

const (
	pollInterval  = time.Second
	flushInterval = 30 * time.Second
	rateHistory   = 300 // seconds of speed kept for the live chart
	liveConnLimit = 300 // connections sent to the page each second
)

// Core counts the traffic of the Clash core into the store and syncs it.
type Core struct {
	dir          string
	settingsPath string
	statePath    string

	store       *store.Store
	sessions    *sessions.Log
	syncer      *syncer.Syncer
	mesh        *mesh.Node
	coll        *collector.Collector // used by the poll loop only
	sub         string               // the subscription in use; poll loop only
	subEndpoint clash.Endpoint       // the core the subscriptions are read for; poll loop only

	mu         sync.Mutex
	file       settingsFile
	client     *clash.Client
	status     ClashStatus
	live       collector.Live
	rates      []RatePoint
	syncStatus SyncStatus

	reconnect chan struct{}
	syncNow   chan struct{}
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	// onTick is called on the poll goroutine with each live snapshot.
	onTick func(collector.Live)
}

// Init loads the settings and opens the store in the user data directory,
// which this process must hold: lockErr is why it could not take it.
func (c *Core) Init(dir string, lockErr error) error {
	if lockErr != nil {
		// Another instance still writes there: two would overwrite each
		// other's counters.
		return i18n.Errorf("Another Clash Traffic uses the data folder (%v). Open this one once it has quit.", lockErr)
	}
	c.dir = dir
	c.settingsPath = filepath.Join(dir, "settings.json")
	c.statePath = filepath.Join(dir, "collector-state.json")
	c.reconnect = make(chan struct{}, 1)
	c.syncNow = make(chan struct{}, 1)

	f, err := loadSettings(c.settingsPath)
	if err != nil {
		log.Println("settings:", err)
	}
	c.file = f
	i18n.Set(languageOf(f.Settings))
	c.store, err = store.Open(filepath.Join(dir, "data"), store.DeviceInfo{
		ID: f.DeviceID, Name: f.DeviceName, OS: runtime.GOOS, Arch: runtime.GOARCH, Version: mygo.App.Version(),
	})
	if err != nil {
		return i18n.Errorf("opening the data folder: %w", err)
	}
	c.syncer = syncer.New(c.store.Root(), f.DeviceID, filepath.Join(dir, "sync-state.json"))
	c.sessions, err = sessions.Open(filepath.Join(dir, "sessions"))
	if err != nil {
		return i18n.Errorf("opening the access log folder: %w", err)
	}
	c.coll = collector.New(c.store)
	c.coll.OnClose = c.sessions.Add
	c.coll.CurrentSub = func() string { return c.sub }

	port := 47321
	if mygo.IsDev() {
		port++ // development builds share with development builds
	}
	c.mesh = mesh.New(c.store.Root(), f.DeviceID, port, filepath.Join(dir, "mesh-state.json"))
	c.mesh.Hello = func() mesh.Hello {
		c.mu.Lock()
		defer c.mu.Unlock()
		return mesh.Hello{
			Device: c.store.Self(),
			Live:   mesh.LiveSummary{Connected: c.status.Connected, UpRate: c.live.UpRate, DownRate: c.live.DownRate},
		}
	}
	c.mesh.Enabled = func() bool { return c.settings().Tailscale }
	c.mesh.OnChange = func(st mesh.MeshStatus) { _ = MeshEvent.Broadcast(st) }
	c.mesh.OnData = func() { _ = DataEvent.Broadcast(DataChange{At: time.Now().UnixMilli(), Reason: "tailscale"}) }
	if err := c.coll.Load(c.statePath, c.store.Gen()); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Stale after a crash between saving the counters and the
		// checkpoint: open connections count from now on instead.
		log.Println("collector state:", err)
	}
	c.syncStatus.Kind = f.SyncKind
	return nil
}

// Start starts counting and syncing.
func (c *Core) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(3)
	go c.pollLoop(ctx)
	go c.syncLoop(ctx)
	go func() {
		defer c.wg.Done()
		c.mesh.Run(ctx)
	}()
}

// Stop saves what was counted and, when syncing is on, sends it to the
// other devices once more.
func (c *Core) Stop() {
	if c.cancel == nil {
		return
	}
	c.cancel()
	c.wg.Wait()
	if c.settings().SyncKind != SyncOff {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_, _ = c.RunSync(ctx)
	}
}

func (c *Core) settings() settingsFile {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.file
}

// relanguage makes the messages of the statuses again, in the language the
// app now shows: those of a failed connection, sync or Tailscale scan.
func (c *Core) relanguage() {
	if !c.Status().Connected {
		c.Reconnect()
	}
	if c.SyncStatus().LastError != "" {
		c.TriggerSync()
	}
	if c.settings().Tailscale && c.mesh != nil {
		c.mesh.Trigger()
	}
}

// Reconnect drops the connection to the core, to find it again.
func (c *Core) Reconnect() {
	select {
	case c.reconnect <- struct{}{}:
	default:
	}
}

// TriggerSync syncs soon, in the background.
func (c *Core) TriggerSync() {
	select {
	case c.syncNow <- struct{}{}:
	default:
	}
}

func (c *Core) pollLoop(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var (
		client     *clash.Client
		nextDetect time.Time
		backoff    = time.Second
		failures   int
		lastFlush  = time.Now()
		lastProc   time.Time
		lastClean  time.Time
		lastSubs   time.Time
	)
	// drop forgets the connection, but not what was counted: if the same
	// core answers again soon, the difference to its totals over the gap is
	// still counted.
	drop := func(why string) {
		if client != nil {
			client.Close()
		}
		client = nil
		c.mu.Lock()
		c.client = nil
		c.mu.Unlock()
		c.setStatus(ClashStatus{Error: why})
	}
	defer func() {
		// Connections still open go into the access log as they are now.
		for _, s := range c.coll.OpenSessions() {
			c.sessions.Add(s)
		}
		c.flush()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.reconnect:
			drop(i18n.T("Reconnecting…"))
			nextDetect, backoff = time.Time{}, time.Second
		case <-ticker.C:
		}
		if time.Since(lastFlush) >= flushInterval {
			c.flush()
			lastFlush = time.Now()
		}
		if time.Since(lastSubs) >= time.Minute {
			lastSubs = time.Now()
			c.refreshSubs()
		}
		if time.Since(lastClean) >= 6*time.Hour {
			lastClean = time.Now()
			if err := c.sessions.Cleanup(c.settings().SessionDays); err != nil {
				log.Println("access log cleanup:", err)
			}
		}

		if client == nil {
			if time.Now().Before(nextDetect) {
				continue
			}
			ep, version, err := c.connect(ctx)
			if err != nil {
				nextDetect = time.Now().Add(backoff)
				backoff = min(backoff*2, 15*time.Second)
				c.setStatus(ClashStatus{Error: err.Error()})
				continue
			}
			backoff, failures, lastProc = time.Second, 0, time.Time{}
			client = clash.NewClient(ep)
			// The collector tells this core from another, and from the one
			// it counted before the app restarted.
			c.coll.Connected(ep.String())
			c.mu.Lock()
			c.client = client
			c.mu.Unlock()
			c.setStatus(ClashStatus{Connected: true, Endpoint: ep.String(), Source: ep.Source, Version: version, Since: time.Now().UnixMilli()})
			// The subscriptions are those of the client this core belongs to.
			c.subEndpoint = ep
			lastSubs = time.Now()
			c.refreshSubs()
		}

		tctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		snap, err := client.Connections(tctx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if failures++; failures >= 3 {
				drop(i18n.T("Lost the connection to Clash: %v", err))
			}
			continue
		}
		failures = 0
		c.publish(c.coll.Tick(time.Now(), snap))

		if time.Since(lastProc) >= 30*time.Second {
			lastProc = time.Now()
			c.checkProcessMode(ctx, client)
		}
	}
}

// connect finds the core as the settings say.
func (c *Core) connect(ctx context.Context) (clash.Endpoint, string, error) {
	f := c.settings()
	return connectWith(ctx, f.ClashMode, f.ClashAddress, f.ClashSecret)
}

func connectWith(ctx context.Context, mode ClashMode, address, secret string) (clash.Endpoint, string, error) {
	if mode != ClashManual {
		return clash.Detect(ctx)
	}
	ep, err := clash.ParseEndpoint(address, secret)
	if err != nil {
		return ep, "", err
	}
	cl := clash.NewClient(ep)
	defer cl.Close()
	tctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	v, err := cl.Version(tctx)
	if err != nil {
		return ep, "", i18n.Errorf("connecting to %s: %w", ep, err)
	}
	return ep, v, nil
}

// refreshSubs reads the Clash client's subscriptions: the one in use names the
// subscription of proxied traffic, and all of them, with their usage, go to
// the other devices. Called on the poll goroutine.
func (c *Core) refreshSubs() {
	profiles, err := clash.Profiles(c.subEndpoint)
	if err != nil {
		log.Println("profiles:", err)
		return
	}
	subs := make([]store.SubInfo, 0, len(profiles))
	c.sub = ""
	for _, p := range profiles {
		if p.Current {
			c.sub = p.Name
		}
		subs = append(subs, store.SubInfo{Name: p.Name, Current: p.Current, Upload: p.Upload, Download: p.Download, Total: p.Total, Expire: p.Expire, Updated: p.Updated})
	}
	if err := c.store.SetSubs(subs); err != nil {
		log.Println("subscriptions:", err)
	}
}

// checkProcessMode keeps find-process-mode at "always" when the settings
// ask for it: Clash Verge sets it back whenever it reloads its profile.
func (c *Core) checkProcessMode(ctx context.Context, client *clash.Client) {
	tctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	mode, err := client.FindProcessMode(tctx)
	if err != nil {
		return
	}
	if c.settings().ForceProcess && mode != "always" {
		if err := client.SetFindProcessMode(tctx, "always"); err != nil {
			log.Println("find-process-mode:", err)
		} else {
			mode = "always"
		}
	}
	c.mu.Lock()
	s := c.status
	c.mu.Unlock()
	if s.Connected && s.FindProcessMode != mode {
		s.FindProcessMode = mode
		c.setStatus(s)
	}
}

func (c *Core) setStatus(s ClashStatus) {
	c.mu.Lock()
	if s.Connected && s.FindProcessMode == "" {
		s.FindProcessMode = c.status.FindProcessMode
	}
	changed := c.status != s
	c.status = s
	c.mu.Unlock()
	if changed {
		_ = StatusEvent.Broadcast(s)
	}
}

// Status returns the state of the connection to the core.
func (c *Core) Status() ClashStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *Core) publish(live collector.Live) {
	c.mu.Lock()
	c.live = live
	c.rates = append(c.rates, RatePoint{T: live.T, Up: live.UpRate, Down: live.DownRate})
	if len(c.rates) > rateHistory {
		c.rates = append(c.rates[:0], c.rates[len(c.rates)-rateHistory:]...)
	}
	c.mu.Unlock()
	if len(live.Conns) > liveConnLimit {
		live.Conns = live.Conns[:liveConnLimit]
	}
	_ = LiveEvent.Broadcast(live)
	if c.onTick != nil {
		c.onTick(live)
	}
}

// Live returns the latest snapshot, with every open connection; the live
// event carries the busiest only.
func (c *Core) Live() collector.Live {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.live
}

// Rates returns the speed of the last minutes.
func (c *Core) Rates() []RatePoint {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]RatePoint(nil), c.rates...)
}

// CloseConnection closes a connection of the core.
func (c *Core) CloseConnection(id string) error {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()
	if client == nil {
		return errors.New(i18n.T("Not connected to Clash"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return client.CloseConnection(ctx, id)
}

// flush saves the counters and what the collector counted of the open
// connections, together. Called on the poll goroutine.
func (c *Core) flush() {
	if err := c.sessions.Flush(); err != nil {
		log.Println("access log:", err)
	}
	if err := c.store.Flush(); err != nil {
		log.Println("flush:", err)
		return
	}
	if err := c.coll.Save(c.statePath, c.store.Gen()); err != nil {
		log.Println("collector state:", err)
	}
}

func (c *Core) syncLoop(ctx context.Context) {
	defer c.wg.Done()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-c.syncNow:
		}
		if c.settings().SyncKind != SyncOff {
			if _, err := c.RunSync(ctx); err != nil {
				log.Println("sync:", err)
			}
		}
		timer.Reset(time.Duration(max(c.settings().SyncMinutes, 1)) * time.Minute)
	}
}

// backendOf returns where the settings sync to.
func backendOf(f settingsFile) (syncer.Backend, error) {
	switch f.SyncKind {
	case SyncFolder:
		if strings.TrimSpace(f.SyncFolder) == "" {
			return nil, errors.New(i18n.T("Choose the sync folder"))
		}
		dir := filepath.Clean(f.SyncFolder)
		if filepath.Base(dir) != "ClashTraffic" {
			dir = filepath.Join(dir, "ClashTraffic")
		}
		return &syncer.Folder{Path: dir}, nil
	case SyncWebDAV:
		if strings.TrimSpace(f.WebDAVURL) == "" {
			return nil, errors.New(i18n.T("Fill in the WebDAV address"))
		}
		return syncer.NewWebDAV(strings.TrimSpace(f.WebDAVURL), f.WebDAVUser, f.webdavPassword(), f.WebDAVDir), nil
	}
	return nil, errors.New(i18n.T("Syncing is off"))
}

// RunSync syncs with the other devices now.
func (c *Core) RunSync(ctx context.Context) (SyncStatus, error) {
	f := c.settings()
	b, err := backendOf(f)
	if err != nil {
		return c.finishSync(f.SyncKind, syncer.Result{}, err), err
	}
	c.mu.Lock()
	c.syncStatus.Kind = f.SyncKind
	c.syncStatus.Running = true
	st := c.syncStatus
	c.mu.Unlock()
	_ = SyncEvent.Broadcast(st)

	// What the poll loop last saved, at most half a minute old: saving here
	// would write the counters without the collector's checkpoint.
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	res, err := c.syncer.Run(ctx, b)
	st = c.finishSync(f.SyncKind, res, err)
	if res.Downloaded > 0 {
		_ = DataEvent.Broadcast(DataChange{At: time.Now().UnixMilli(), Reason: "sync"})
	}
	return st, err
}

func (c *Core) finishSync(kind SyncKind, res syncer.Result, err error) SyncStatus {
	c.mu.Lock()
	s := &c.syncStatus
	s.Kind, s.Running = kind, false
	s.LastSync = time.Now().UnixMilli()
	s.Uploaded, s.Downloaded, s.Devices = res.Uploaded, res.Downloaded, res.Devices
	s.LastError = ""
	if err != nil {
		s.LastError = err.Error()
	}
	st := *s
	c.mu.Unlock()
	_ = SyncEvent.Broadcast(st)
	return st
}

// SyncStatus returns how syncing went last.
func (c *Core) SyncStatus() SyncStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.syncStatus
}

// Settings returns the user's preferences.
func (c *Core) Settings() Settings {
	f := c.settings()
	s := f.Settings
	s.OpenAtLogin = mygo.App.OpenAtLogin()
	s.HasWebDAVPassword = f.webdavPassword() != ""
	s.AutoCheckUpdates, s.AutoInstallUpdates = updatePrefs()
	return s
}

// SaveSettings applies and saves new preferences. A nil password keeps
// the saved one.
func (c *Core) SaveSettings(in Settings, password *string) (Settings, error) {
	c.mu.Lock()
	old := c.file
	f := old
	in.DeviceID = old.DeviceID
	in.DeviceName = strings.TrimSpace(in.DeviceName)
	if in.DeviceName == "" {
		in.DeviceName = old.DeviceName
	}
	if in.ClashMode != ClashManual {
		in.ClashMode = ClashAuto
	}
	switch in.SyncKind {
	case SyncFolder, SyncWebDAV:
	default:
		in.SyncKind = SyncOff
	}
	in.SyncMinutes = min(max(in.SyncMinutes, 1), 24*60)
	if in.SessionDays < 0 {
		in.SessionDays = 30
	}
	in.WebDAVDir = strings.Trim(strings.TrimSpace(in.WebDAVDir), "/")
	if in.WebDAVDir == "" {
		in.WebDAVDir = "ClashTraffic"
	}
	if !i18n.Supported(in.Language) {
		in.Language = ""
	}
	f.Settings = in
	if password != nil {
		f.setWebDAVPassword(*password)
	}
	if err := saveSettings(c.settingsPath, f); err != nil {
		c.mu.Unlock()
		return c.Settings(), err
	}
	c.file = f
	c.syncStatus.Kind = in.SyncKind
	c.mu.Unlock()

	var errs []error
	setUpdatePrefs(in.AutoCheckUpdates, in.AutoInstallUpdates)
	if in.OpenAtLogin != mygo.App.OpenAtLogin() {
		if err := mygo.App.SetOpenAtLogin(in.OpenAtLogin); err != nil {
			errs = append(errs, i18n.Errorf("opening at login: %w", err))
		}
	}
	if in.DeviceName != old.DeviceName {
		if err := c.store.Rename(in.DeviceName); err != nil {
			errs = append(errs, err)
		}
	}
	if in.ClashMode != old.ClashMode || in.ClashAddress != old.ClashAddress || in.ClashSecret != old.ClashSecret || in.ForceProcess != old.ForceProcess {
		c.Reconnect()
	}
	if in.SyncKind != SyncOff && (in.SyncKind != old.SyncKind || in.SyncFolder != old.SyncFolder ||
		in.WebDAVURL != old.WebDAVURL || in.WebDAVUser != old.WebDAVUser || in.WebDAVDir != old.WebDAVDir || password != nil) {
		c.TriggerSync()
	}
	if in.DeviceName != old.DeviceName {
		_ = DataEvent.Broadcast(DataChange{At: time.Now().UnixMilli(), Reason: "device"})
	}
	if in.Tailscale != old.Tailscale {
		c.mesh.Trigger()
	}
	if in.Language != old.Language {
		setLanguage(languageOf(in))
	}
	if in.SessionDays != old.SessionDays {
		if err := c.sessions.Cleanup(in.SessionDays); err != nil {
			errs = append(errs, err)
		}
	}
	return c.Settings(), errors.Join(errs...)
}

// TestSync checks settings that are not saved yet.
func (c *Core) TestSync(in Settings, password *string) error {
	f := c.settings()
	f.Settings = in
	if password != nil {
		f.WebDAVPassword = *password
	}
	b, err := backendOf(f)
	if err != nil {
		return err
	}
	return syncer.Check(context.Background(), b, f.DeviceID)
}

// Sessions returns a page of the access log, with the connections open now
// as the core reports them.
func (c *Core) Sessions(q sessions.SessionQuery) sessions.SessionPage {
	c.mu.Lock()
	live := c.live
	c.mu.Unlock()
	open := make([]sessions.Session, 0, len(live.Conns))
	for _, lc := range live.Conns {
		open = append(open, sessions.Session{
			ID: lc.ID, Start: lc.Start, End: live.T, Open: true,
			App: lc.App, Process: lc.Process, Host: lc.Host, Dest: lc.Dest, Source: lc.Source,
			Network: lc.Network, Inbound: lc.Inbound, Chain: lc.Chain, Chains: lc.Chains, Route: lc.Route, Sub: lc.Sub, Rule: lc.Rule,
			Up: lc.Up, Down: lc.Down,
		})
	}
	return c.sessions.Query(q, open)
}
