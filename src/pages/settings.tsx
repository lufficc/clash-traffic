import { useEffect, useRef, useState } from "preact/hooks";
import type { ComponentChildren } from "preact";
import { Card, type PageProps } from "../app";
import { errorText } from "../lib/hooks";
import { ago } from "../lib/format";
import { takeChanged } from "../lib/form";
import { LANGUAGES, t, tn, tx } from "../lib/i18n";
import { events, Monitor, Preferences, type AboutInfo, type ClashStatus, type Endpoint, type Settings } from "../mygo";

export function SettingsPage({ status, notify }: PageProps) {
  const [saved, setSaved] = useState<Settings | null>(null);
  const [form, setForm] = useState<Settings | null>(null);
  const [password, setPassword] = useState<string | null>(null);
  const [about, setAbout] = useState<AboutInfo | null>(null);
  const [candidates, setCandidates] = useState<Endpoint[] | null>(null);
  const [clashTest, setClashTest] = useState<ClashStatus | null>(null);
  const [syncTest, setSyncTest] = useState<{ ok: boolean; msg: string } | null>(null);
  const [busy, setBusy] = useState("");
  const savedRef = useRef(saved);
  savedRef.current = saved;

  useEffect(() => {
    Preferences.get().then((s) => (setSaved(s), setForm(s))).catch((err) => notify(errorText(err), "error"));
    Preferences.about().then(setAbout).catch(() => {});
    // Texts made in the old language go: why updates cannot install, and
    // the results of the tests.
    const offLanguage = events.language.on(() => {
      Preferences.about().then(setAbout).catch(() => {});
      setClashTest(null);
      setSyncTest(null);
    });
    // The update window changes these too, and each check sends them: an
    // unsaved edit of one stays.
    const offUpdates = events.updates.on((u) => {
      const changed = { autoCheckUpdates: u.autoCheck, autoInstallUpdates: u.autoInstall };
      const before = savedRef.current;
      setAbout((a) => a && { ...a, updates: u });
      setSaved((s) => s && { ...s, ...changed });
      setForm((f) => f && before && takeChanged(f, before, changed));
    });
    return () => {
      offLanguage();
      offUpdates();
    };
  }, []);
  if (!form || !saved) return null;

  const set = (patch: Partial<Settings>) => setForm({ ...form, ...patch });
  const dirty = JSON.stringify(form) !== JSON.stringify(saved) || password !== null;
  const isMac = document.documentElement.dataset.platform === "darwin";
  const isWindows = document.documentElement.dataset.platform === "win32";
  const updates = about?.updates;
  const canUpdate = !!updates?.enabled;

  const run = async (name: string, fn: () => Promise<void>) => {
    setBusy(name);
    try {
      await fn();
    } catch (err) {
      notify(errorText(err), "error");
    } finally {
      setBusy("");
    }
  };
  const save = () =>
    run("save", async () => {
      try {
        const s = await Preferences.save(form, password);
        setSaved(s);
        setForm(s);
        setPassword(null);
        notify(t("Settings saved"));
      } catch (err) {
        // Saved, but something did not apply, such as starting at login.
        const s = await Preferences.get();
        setSaved(s);
        setForm(s);
        throw err;
      }
    });
  const testClash = () => run("clash", async () => setClashTest(await Preferences.testClash(form.clashMode, form.clashAddress, form.clashSecret)));
  const testSync = () =>
    run("sync", async () => {
      try {
        await Preferences.testSync(form, password);
        setSyncTest({ ok: true, msg: t("Reads and writes fine") });
      } catch (err) {
        setSyncTest({ ok: false, msg: errorText(err) });
      }
    });
  const chooseFolder = () =>
    run("folder", async () => {
      const dir = await Preferences.chooseFolder();
      if (dir) set({ syncFolder: dir });
    });

  return (
    <div class="settings">
      <Card title={t("Clash connection")}>
        <div class="status-box">
          <span class={`status-dot ${status?.connected ? "good" : "critical"}`} aria-hidden="true" />
          <div>
            <strong>{status?.connected ? `${t("Connected")} · mihomo ${status.version}` : t("Not connected")}</strong>
            <div class="muted small">{status?.connected ? t("{endpoint} (from {source})", { endpoint: status.endpoint, source: sourceLabel(status.source) }) : status?.error}</div>
          </div>
          <button type="button" class="ghost small" onClick={() => Monitor.reconnect()}>
            {t("Reconnect")}
          </button>
        </div>
        <Field label={t("Connect")}>
          <div class="segmented">
            <button type="button" class={form.clashMode === "auto" ? "on" : ""} onClick={() => set({ clashMode: "auto" })}>
              {t("Find Clash Verge")}
            </button>
            <button type="button" class={form.clashMode === "manual" ? "on" : ""} onClick={() => set({ clashMode: "manual" })}>
              {t("By hand")}
            </button>
          </div>
        </Field>
        {form.clashMode === "manual" ? (
          <>
            <Field label={t("Controller address")} hint={t("Such as {examples}", { examples: "http://127.0.0.1:9097, unix:///tmp/verge/verge-mihomo.sock, \\\\.\\pipe\\verge-mihomo" })}>
              <input type="text" value={form.clashAddress} placeholder="http://127.0.0.1:9097" onInput={(e) => set({ clashAddress: e.currentTarget.value })} />
            </Field>
            <Field label={t("Secret")} hint={t("The secret of Clash Verge's external controller setting; not needed with a socket or pipe")}>
              <input type="password" value={form.clashSecret} onInput={(e) => set({ clashSecret: e.currentTarget.value })} />
            </Field>
          </>
        ) : (
          <Field label={t("Search order")} hint={isWindows ? t("Tries in turn: Clash Verge's config files → the pipes of running cores → the default addresses") : t("Tries in turn: the running core's command line → Clash Verge's config files → the default addresses")}>
            {candidates ? (
              <ul class="plain-list">
                {candidates.map((c) => (
                  <li key={c.address}>
                    <code>{c.kind === "unix" ? `unix://${c.address}` : c.address}</code> <span class="muted">({sourceLabel(c.source)})</span>
                  </li>
                ))}
                {!candidates.length && <li class="muted">{t("No address found: Clash Verge may not be running")}</li>}
              </ul>
            ) : (
              <button type="button" class="link" onClick={() => Monitor.candidates().then(setCandidates)}>
                {t("Show the addresses")}
              </button>
            )}
          </Field>
        )}
        <div class="row">
          <button type="button" class="ghost" disabled={busy === "clash"} onClick={testClash}>
            {busy === "clash" ? t("Testing…") : t("Test the connection")}
          </button>
          {clashTest && (
            <span class={clashTest.connected ? "ok-text" : "error-text"}>
              {clashTest.connected
                ? `✓ ${t("Connected to {endpoint} (mihomo {version})", { endpoint: clashTest.endpoint, version: clashTest.version })}`
                : `✕ ${clashTest.error}`}
            </span>
          )}
        </div>
        <Toggle checked={form.forceProcess} onChange={(v) => set({ forceProcess: v })} label={t("Turn on process identification (needed to count each app)")}>
          {tx(
            "With Clash Verge's default {strict}, the core identifies processes only when a rule needs them, so most connections come from no known app. This turns the running core to {always}, again after Clash Verge reloads its config.",
            { strict: <code>find-process-mode: strict</code>, always: <code>always</code> },
          )}{" "}
          {tx("The core now: {mode}. To keep it for good, add {line} to Clash Verge's global extend config, under Profiles.", {
            mode: <code>{status?.findProcessMode || t("Unknown")}</code>,
            line: <code>find-process-mode: always</code>,
          })}
        </Toggle>
      </Card>

      <Card title={t("This computer")}>
        <Field label={t("Device name")} hint={t("Shown in the numbers of every device")}>
          <input type="text" value={form.deviceName} onInput={(e) => set({ deviceName: e.currentTarget.value })} />
        </Field>
        <Field label={t("Language")}>
          <select value={form.language} onChange={(e) => set({ language: e.currentTarget.value })}>
            <option value="">{t("Same as the system")}</option>
            {LANGUAGES.map((l) => (
              <option key={l.id} value={l.id}>
                {l.name}
              </option>
            ))}
          </select>
        </Field>
        <Toggle checked={form.openAtLogin} onChange={(v) => set({ openAtLogin: v })} label={t("Open at login")}>
          {isMac ? t("Counts in the background without opening its window; open it from the menu bar icon.") : t("Counts in the background without opening its window; open it from the tray icon.")}
        </Toggle>
        {isMac && <Toggle checked={form.traySpeed} onChange={(v) => set({ traySpeed: v })} label={t("Show the download speed in the menu bar")} />}
        {isMac && (
          <Toggle checked={form.hideDock} onChange={(v) => set({ hideDock: v })} label={t("Hide the Dock icon when the window is closed")}>
            {t("Keeps counting in the menu bar only; open the window from the menu bar icon.")}
          </Toggle>
        )}
      </Card>

      <Card title={t("Multi-device sync")}>
        <Toggle checked={form.tailscale} onChange={(v) => set({ tailscale: v })} label={t("Find and sync over Tailscale (recommended)")}>
          {t("No setup: the computers of the tailnet that run Clash Traffic find each other, fetch each other's numbers every minute and show them together.")}{" "}
          {t("The data goes straight between your own devices, through no third party; devices shared into the tailnet by others cannot read it; the access log is not shared.")}
        </Toggle>
        <p class="muted small sub-head">{t("Other ways to sync (optional): for devices that are not online at the same time, exchange the data through a shared folder or WebDAV too.")}</p>
        <Field label={t("Sync with")}>
          <div class="segmented">
            {(
              [
                ["off", "Off"],
                ["folder", "Shared folder"],
                ["webdav", "WebDAV"],
              ] as const
            ).map(([k, label]) => (
              <button type="button" key={k} class={form.syncKind === k ? "on" : ""} onClick={() => set({ syncKind: k })}>
                {t(label)}
              </button>
            ))}
          </div>
        </Field>
        {form.syncKind === "folder" && (
          <Field label={t("Folder")} hint={t("A folder of iCloud Drive, OneDrive, Dropbox, Syncthing or a network share: a ClashTraffic folder is made in it")}>
            <div class="row">
              <input type="text" value={form.syncFolder} placeholder={t("None chosen")} onInput={(e) => set({ syncFolder: e.currentTarget.value })} />
              <button type="button" class="ghost" onClick={chooseFolder}>
                {t("Choose…")}
              </button>
            </div>
          </Field>
        )}
        {form.syncKind === "webdav" && (
          <>
            <Field label={t("Server")} hint={t("Nutstore (坚果云): https://dav.jianguoyun.com/dav/; Nextcloud, Synology, Alist and others work too")}>
              <input type="url" value={form.webdavUrl} onInput={(e) => set({ webdavUrl: e.currentTarget.value })} />
            </Field>
            <Field label={t("User name")}>
              <input type="text" value={form.webdavUser} autoComplete="off" onInput={(e) => set({ webdavUser: e.currentTarget.value })} />
            </Field>
            <Field label={t("Password")} hint={t("Nutstore needs an app password, made under Account → Security")}>
              <input
                type="password"
                value={password ?? ""}
                placeholder={saved.hasWebdavPassword ? t("Saved (leave empty to keep it)") : ""}
                autoComplete="new-password"
                onInput={(e) => setPassword(e.currentTarget.value === "" && !saved.hasWebdavPassword ? null : e.currentTarget.value)}
              />
            </Field>
            <Field label={t("Folder on the server")}>
              <input type="text" value={form.webdavDir} onInput={(e) => set({ webdavDir: e.currentTarget.value })} />
            </Field>
          </>
        )}
        {form.syncKind !== "off" && (
          <>
            <Field label={t("Sync every")}>
              <select value={String(form.syncMinutes)} onChange={(e) => set({ syncMinutes: Number(e.currentTarget.value) })}>
                {[1, 5, 10, 30, 60].map((m) => (
                  <option key={m} value={m}>
                    {tn("Every {n} minutes", m)}
                  </option>
                ))}
              </select>
            </Field>
            <div class="row">
              <button type="button" class="ghost" disabled={busy === "sync"} onClick={testSync}>
                {busy === "sync" ? t("Testing…") : t("Test syncing")}
              </button>
              {syncTest && <span class={syncTest.ok ? "ok-text" : "error-text"}>{syncTest.ok ? `✓ ${syncTest.msg}` : `✕ ${syncTest.msg}`}</span>}
            </div>
          </>
        )}
      </Card>

      <Card title={t("Data")}>
        <Field label={t("Keep the access log")} hint={t("The log of each connection stays on this computer; the hourly numbers are kept for good either way")}>
          <select value={String(form.sessionDays)} onChange={(e) => set({ sessionDays: Number(e.currentTarget.value) })}>
            {(
              [
                [7, tn("{n} days", 7)],
                [30, tn("{n} days", 30)],
                [90, tn("{n} days", 90)],
                [365, t("1 year")],
                [0, t("Forever")],
              ] as const
            ).map(([v, label]) => (
              <option key={v} value={v}>
                {label}
              </option>
            ))}
          </select>
        </Field>
        <dl class="kv">
          <dt>{t("Data folder")}</dt>
          <dd>
            <code>{about?.dataDir}</code>{" "}
            <button type="button" class="link" onClick={() => Preferences.openDataFolder().catch((err) => notify(errorText(err), "error"))}>
              {t("Show")}
            </button>
          </dd>
          <dt>{t("Device ID")}</dt>
          <dd>
            <code>{about?.deviceId}</code>
          </dd>
        </dl>
      </Card>

      <Card title={t("Software update")}>
        <div class="status-box">
          <div>
            <strong>
              Clash Traffic {about?.version || t("development build")} <span class="muted small">{about?.platform}</span>
            </strong>
            <div class="muted small">{updates?.enabled ? t("Last checked: {ago}", { ago: ago(updates.lastCheck) }) : updates?.reason}</div>
          </div>
          <button type="button" class="ghost small" onClick={() => Preferences.checkForUpdates().catch((err) => notify(errorText(err), "error"))}>
            {t("Check for updates…")}
          </button>
        </div>
        <Toggle checked={form.autoCheckUpdates} disabled={!canUpdate} onChange={(v) => set({ autoCheckUpdates: v })} label={t("Check for updates automatically")}>
          {t("Looks for a new version on GitHub once a day and shows its release notes; you choose whether to install it.")}
        </Toggle>
        <Toggle
          checked={form.autoInstallUpdates}
          disabled={!canUpdate || !form.autoCheckUpdates}
          onChange={(v) => set({ autoInstallUpdates: v })}
          label={t("Download and install updates automatically")}
        >
          {t("Without asking, downloads and installs new versions in the background; they run the next time the app starts.")}
        </Toggle>
      </Card>

      <div class={`savebar${dirty ? " show" : ""}`}>
        <span>{t("Unsaved changes")}</span>
        <button type="button" class="ghost" onClick={() => (setForm(saved), setPassword(null))}>
          {t("Revert")}
        </button>
        <button type="button" class="primary" disabled={busy === "save"} onClick={save}>
          {busy === "save" ? t("Saving…") : t("Save")}
        </button>
      </div>
    </div>
  );
}

// Where Go found a controller, in English, or the name of a config file.
const SOURCES = ["default", "the running core", "set by hand"];
function sourceLabel(source: string): string {
  return SOURCES.includes(source) ? t(source) : source;
}

function Field({ label, hint, children }: { label: string; hint?: string; children: ComponentChildren }) {
  return (
    <div class="field" role="group" aria-label={label}>
      <span class="field-label">{label}</span>
      <span class="field-body">
        {children}
        {hint && <span class="hint">{hint}</span>}
      </span>
    </div>
  );
}

function Toggle({
  checked,
  disabled,
  onChange,
  label,
  children,
}: {
  checked: boolean;
  disabled?: boolean;
  onChange: (v: boolean) => void;
  label: string;
  children?: ComponentChildren;
}) {
  return (
    <div class={`toggle-row${disabled ? " disabled" : ""}`}>
      <label class="switch big">
        <input type="checkbox" checked={checked} disabled={disabled} onChange={(e) => onChange(e.currentTarget.checked)} />
        <span>{label}</span>
      </label>
      {children && <p class="hint">{children}</p>}
    </div>
  );
}
