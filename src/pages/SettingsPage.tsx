import { useEffect, useId, useState } from "react";
import {
  Button,
  FormControl,
  IconButton,
  Label,
  NavList,
  Select,
  TextInput,
  ToggleSwitch,
} from "@primer/react";
import {
  ChevronLeftIcon,
  CopyIcon,
  FileDirectoryIcon,
  GlobeIcon,
  LinkExternalIcon,
  SyncIcon,
} from "@primer/octicons-react";
import { Settings as SettingsAPI, System } from "../mygo";
import { useApp, type SettingsSection } from "../store/useApp";
import { categories, strings } from "../lib/strings";

const sections: { id: SettingsSection; label: string }[] = [
  { id: "general", label: strings.sectionGeneral },
  { id: "download", label: strings.sectionDownload },
  { id: "category", label: strings.sectionCategory },
  { id: "browser", label: strings.sectionBrowser },
  { id: "about", label: strings.sectionAbout },
];

const folderNames: Record<string, string> = {
  video: "Video",
  audio: "Audio",
  archive: "Archives",
  document: "Documents",
  app: "Apps",
  other: "Other",
};

/** SettingsPage is the whole page behind 设置: every change is saved at once. */
export function SettingsPage() {
  const closeSettings = useApp((s) => s.closeSettings);
  const section = useApp((s) => s.settingsSection);
  const setSection = useApp((s) => s.setSettingsSection);
  const settings = useApp((s) => s.settings);
  const browser = useApp((s) => s.browser);
  const save = useApp((s) => s.saveSettings);
  const refreshBrowser = useApp((s) => s.refreshBrowser);
  const setBrowserEnabled = useApp((s) => s.setBrowserEnabled);
  const installBrowserHost = useApp((s) => s.installBrowserHost);
  const openExtensionPage = useApp((s) => s.openExtensionPage);
  const toast = useApp((s) => s.toast);
  const [version, setVersion] = useState("");
  const [logsDir, setLogsDir] = useState("");
  const megabytes = (bytes: number) => Math.round((bytes / (1024 * 1024)) * 10) / 10;

  useEffect(() => {
    void refreshBrowser();
    System.version().then(setVersion).catch(() => setVersion(""));
    System.logsDir().then(setLogsDir).catch(() => setLogsDir(""));
  }, [refreshBrowser]);

  if (!settings) {
    return (
      <section className="settings">
        <p className="settings-loading">{strings.loading}</p>
      </section>
    );
  }

  const allBrowsers = browser?.browsers ?? [];
  const detectedBrowsers = allBrowsers.filter((b) => b.installed);
  const selectedBrowser = settings.browserHost === "auto" ? undefined : allBrowsers.find((b) => b.id === settings.browserHost);

  return (
    <section className="settings">
      <aside className="settings-nav">
        <button type="button" className="settings-back" onClick={closeSettings}>
          <ChevronLeftIcon size={14} /> {strings.settingsBack}
        </button>
        <NavList aria-label="设置分类">
          {sections.map((s) => (
            <NavList.Item
              key={s.id}
              as="button"
              type="button"
              aria-current={section === s.id ? "page" : undefined}
              onClick={() => setSection(s.id)}
            >
              {s.label}
            </NavList.Item>
          ))}
        </NavList>
      </aside>

      <div className="settings-body">
        <h2>{sections.find((s) => s.id === section)?.label}</h2>

        {section === "general" && (
          <>
            <div className="setting-row">
              <p className="setting-title">{strings.generalTheme}</p>
              <FormControl>
                <Select
                  value={settings.theme}
                  aria-label={strings.generalTheme}
                  onChange={(event) => void save({ theme: event.target.value })}
                >
                  <Select.Option value="system">{strings.themeSystem}</Select.Option>
                  <Select.Option value="dark">{strings.themeDark}</Select.Option>
                  <Select.Option value="light">{strings.themeLight}</Select.Option>
                </Select>
              </FormControl>
            </div>
            <SettingSwitch
              label={strings.closeToTray}
              checked={settings.closeToTray}
              onChange={(closeToTray) => void save({ closeToTray })}
            />
            <SettingSwitch
              label={strings.notifyComplete}
              checked={settings.notifyOnComplete}
              onChange={(notifyOnComplete) => void save({ notifyOnComplete })}
            />
            <SettingSwitch
              label={strings.openAtLogin}
              checked={settings.openAtLogin}
              onChange={(openAtLogin) => void save({ openAtLogin })}
            />
            <SettingSwitch
              label={strings.autoResume}
              checked={settings.autoResume}
              onChange={(autoResume) => void save({ autoResume })}
            />
          </>
        )}

        {section === "download" && (
          <>
            <div className="setting-row setting-row-block">
              <p className="setting-title">{strings.downloadDir}</p>
              <div className="field-with-button">
                <TextInput block value={settings.downloadDir} onChange={(event) => void save({ downloadDir: event.target.value })} />
                <Button
                  leadingVisual={FileDirectoryIcon}
                  onClick={async () => {
                    const dir = await SettingsAPI.chooseDir();
                    if (dir) void save({ downloadDir: dir });
                  }}
                >
                  {strings.addChoose}
                </Button>
              </div>
            </div>
            <SettingRange
              label={strings.connections}
              min={1}
              max={32}
              value={settings.connections}
              onChange={(connections) => void save({ connections })}
            />
            <SettingRange
              label={strings.maxConcurrent}
              min={1}
              max={8}
              value={settings.maxConcurrent}
              onChange={(maxConcurrent) => void save({ maxConcurrent })}
            />
            <SettingRange
              label={`${strings.minSegment}（MiB）`}
              hint={strings.minSegmentHint}
              min={0.25}
              max={16}
              step={0.25}
              value={megabytes(settings.minSegmentBytes)}
              onChange={(value) => void save({ minSegmentBytes: Math.round(value * 1024 * 1024) })}
            />
            <SettingRange
              label={`${strings.speedLimit}（MB/s）`}
              hint={strings.speedLimitHint}
              min={0}
              max={50}
              value={Math.round(settings.speedLimit / 1_000_000)}
              onChange={(value) => void save({ speedLimit: Math.round(value * 1_000_000) })}
            />
            <SettingRange
              label={strings.maxRetries}
              min={1}
              max={20}
              value={settings.maxRetries}
              onChange={(maxRetries) => void save({ maxRetries })}
            />
          </>
        )}

        {section === "category" && (
          <>
            <SettingSwitch
              label={strings.categorize}
              hint={strings.categorizeHint}
              checked={settings.categorize}
              onChange={(categorize) => void save({ categorize })}
            />
            <table className="segments" style={{ marginTop: "var(--base-size-16)" }}>
              <thead>
                <tr>
                  <th>类型</th>
                  <th>{strings.categoryMap}</th>
                </tr>
              </thead>
              <tbody>
                {categories.map((c) => (
                  <tr key={c.id}>
                    <td>{c.label}</td>
                    <td className="mono">{folderNames[c.id] ?? "Other"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}

        {section === "browser" && (
          <>
            <SettingSwitch
              label={strings.browserEnabled}
              hint={strings.browserHint}
              checked={settings.browserEnabled}
              onChange={(enabled) => void setBrowserEnabled(enabled)}
            />
            <div className="setting-row">
              <div>
                <p className="setting-title">{strings.browserHost}</p>
                <span className="setting-hint">{strings.browserHostHint}</span>
              </div>
              <FormControl>
                <Select
                  value={settings.browserHost}
                  aria-label={strings.browserHost}
                  onChange={async (event) => {
                    // Changing the host re-registers the native host, so
                    // refresh the detected list afterwards.
                    await save({ browserHost: event.target.value });
                    await refreshBrowser();
                  }}
                >
                  <Select.Option value="auto">{strings.browserHostAuto}</Select.Option>
                  {(browser?.browsers ?? [])
                    .filter((b) => b.installed)
                    .map((b) => (
                      <Select.Option key={b.id} value={b.id}>
                        {b.name}
                      </Select.Option>
                    ))}
                  {settings.browserHost !== "auto" && !selectedBrowser?.installed && (
                    <Select.Option value={settings.browserHost}>
                      {selectedBrowser ? `${selectedBrowser.name}（${strings.browserNotInstalled}）` : settings.browserHost}
                    </Select.Option>
                  )}
                </Select>
              </FormControl>
            </div>
            <div className="setting-row setting-row-block">
              <p className="setting-title">{strings.browserDetected}</p>
              {detectedBrowsers.length === 0 ? (
                <p className="setting-hint">{strings.browserNone}</p>
              ) : (
                <div className="status-row">
                  {detectedBrowsers.map((b) => (
                    <Label key={b.id} variant={b.registered ? "success" : "secondary"}>
                      {`${b.name} · ${b.registered ? strings.browserRegistered : strings.browserNotRegistered}`}
                    </Label>
                  ))}
                </div>
              )}
            </div>
            <div className="status-row">
              <Label variant={browser?.hostInstalled ? "success" : "danger"}>
                {browser?.hostInstalled ? strings.hostInstalled : strings.hostMissing}
              </Label>
              <Label variant={browser?.connected ? "success" : "secondary"}>
                {browser?.connected ? strings.connected : strings.disconnected}
              </Label>
            </div>
            <div className="setting-row setting-row-block">
              <p className="setting-title">{strings.extensionId}</p>
              <div className="field-with-button">
                <TextInput block readOnly value={browser?.extensionId ?? "—"} aria-label={strings.extensionId} />
                <IconButton
                  icon={CopyIcon}
                  aria-label={strings.copyId}
                  title={strings.copyId}
                  disabled={!browser?.extensionId}
                  onClick={async () => {
                    await System.writeClipboardText(browser?.extensionId ?? "");
                    toast("success", strings.copied);
                  }}
                />
              </div>
            </div>
            <div className="setting-row setting-row-block">
              <p className="setting-title">{strings.extensionDir}</p>
              <p className="setting-value mono">{browser?.extensionDir || "—"}</p>
            </div>
            <div className="settings-actions">
              <Button
                variant="primary"
                leadingVisual={SyncIcon}
                onClick={async () => {
                  // The store installs the host and then loads the unpacked
                  // extension into browsers that are not running.
                  await installBrowserHost();
                }}
              >
                {strings.installIntegration}
              </Button>
              <Button
                leadingVisual={GlobeIcon}
                onClick={async () => {
                  const opened = await openExtensionPage();
                  if (opened && opened.length > 0) toast("success", `${strings.openExtensions}：${opened.join("、")}`);
                }}
              >
                {strings.openExtensions}
              </Button>
              <Button
                leadingVisual={LinkExternalIcon}
                disabled={!browser?.extensionId}
                onClick={() =>
                  void System.openExternal(`chrome-extension://${browser?.extensionId ?? ""}/options.html`).catch(() =>
                    toast("error", strings.loadFailed),
                  )
                }
              >
                {strings.openExtensionOptions}
              </Button>
              <Button
                leadingVisual={FileDirectoryIcon}
                onClick={() => browser?.extensionDir && void System.showInFolder(browser.extensionDir)}
              >
                {strings.showExtensionDir}
              </Button>
            </div>
            <p className="setting-hint">{strings.extensionHint}</p>
          </>
        )}

        {section === "about" && (
          <>
            <div className="setting-row">
              <p className="setting-title">{strings.version}</p>
              <p className="setting-value mono">{version || "—"}</p>
            </div>
            <div className="setting-row">
              <p className="setting-title">{strings.logs}</p>
              <Button leadingVisual={FileDirectoryIcon} onClick={() => logsDir && void System.showInFolder(logsDir)}>
                {logsDir || "—"}
              </Button>
            </div>
            <div className="setting-row">
              <p className="setting-title">{strings.homepage}</p>
              <Button
                leadingVisual={LinkExternalIcon}
                onClick={() => void System.openExternal("https://github.com/").catch(() => toast("error", strings.loadFailed))}
              >
                {strings.homepage}
              </Button>
            </div>
          </>
        )}
      </div>
    </section>
  );
}

function SettingSwitch({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string;
  hint?: string;
  checked: boolean;
  onChange: (value: boolean) => void;
}) {
  const id = useId();
  return (
    <div className="setting-row">
      <div>
        <p className="setting-title" id={id}>
          {label}
        </p>
        {hint && <span className="setting-hint">{hint}</span>}
      </div>
      <ToggleSwitch
        aria-labelledby={id}
        checked={checked}
        onChange={onChange}
        buttonLabelOn="开"
        buttonLabelOff="关"
      />
    </div>
  );
}

function SettingRange({
  label,
  hint,
  min,
  max,
  step = 1,
  value,
  onChange,
}: {
  label: string;
  hint?: string;
  min: number;
  max: number;
  step?: number;
  value: number;
  onChange: (value: number) => void;
}) {
  const id = useId();
  return (
    <div className="setting-range">
      <div className="setting-range-head">
        <label className="setting-title" htmlFor={id}>
          {label}
        </label>
        <span className="setting-value num">{value}</span>
      </div>
      <input
        id={id}
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
      />
      {hint && <span className="setting-hint">{hint}</span>}
    </div>
  );
}
