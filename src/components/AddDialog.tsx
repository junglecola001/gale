import { useEffect, useState } from "react";
import { Button, Dialog, FormControl, Select, Textarea, TextInput } from "@primer/react";
import { ChevronDownIcon, FileDirectoryIcon } from "@primer/octicons-react";
import { Settings as SettingsAPI } from "../mygo";
import type { AddRequest } from "../mygo";
import { useApp } from "../store/useApp";
import { formatBytes } from "../lib/format";
import { categories, strings } from "../lib/strings";

/** AddDialog asks for a URL and how to download it. */
export function AddDialog() {
  const open = useApp((s) => s.addOpen);
  const initialUrl = useApp((s) => s.addUrl);
  const preview = useApp((s) => s.addPreview);
  const settings = useApp((s) => s.settings);
  const closeAdd = useApp((s) => s.closeAdd);
  const doPreview = useApp((s) => s.preview);
  const addDownload = useApp((s) => s.addDownload);

  const [url, setUrl] = useState("");
  const [name, setName] = useState("");
  const [dir, setDir] = useState("");
  const [category, setCategory] = useState("");
  const [connections, setConnections] = useState(8);
  const [advanced, setAdvanced] = useState(false);
  const [referer, setReferer] = useState("");
  const [userAgent, setUserAgent] = useState("");
  const [cookies, setCookies] = useState("");
  const [busy, setBusy] = useState(false);
  const [parsing, setParsing] = useState(false);

  useEffect(() => {
    if (!open) return;
    setUrl(initialUrl);
    setName("");
    setCategory("");
    setConnections(settings?.connections ?? 8);
    setDir(settings?.downloadDir ?? "");
    setAdvanced(false);
    setReferer("");
    setUserAgent("");
    setCookies("");
    setBusy(false);
  }, [open, initialUrl, settings]);

  const request = (start: boolean): AddRequest => ({
    url: url.trim(),
    dir,
    fileName: name.trim(),
    referer: referer.trim(),
    userAgent: userAgent.trim(),
    cookies: cookies.trim(),
    category,
    connections,
    start,
  });

  const parse = async () => {
    const target = url.trim();
    if (!target) return;
    setParsing(true);
    const result = await doPreview(request(false));
    setParsing(false);
    if (result && !name.trim()) setName(result.fileName);
  };

  const submit = async (start: boolean) => {
    if (!url.trim() || busy) return;
    setBusy(true);
    const ok = await addDownload(request(start));
    setBusy(false);
    if (ok) closeAdd();
  };

  if (!open) return null;

  return (
    <Dialog
      title={strings.add}
      width="large"
      onClose={() => closeAdd()}
      footerButtons={[
        { content: parsing ? strings.addParsing : strings.addParse, onClick: () => void parse(), disabled: !url.trim() || parsing },
        { content: strings.addLater, onClick: () => void submit(false), disabled: !url.trim() || busy },
        { content: strings.addStart, buttonType: "primary", onClick: () => void submit(true), disabled: !url.trim() || busy },
      ]}
    >
      <div className="dialog-form">
        <FormControl>
          <FormControl.Label>{strings.addUrl}</FormControl.Label>
          <Textarea
            block
            rows={2}
            value={url}
            placeholder={strings.addUrlPlaceholder}
            autoFocus
            onChange={(event) => setUrl(event.target.value)}
            onBlur={() => void parse()}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                void submit(true);
              }
            }}
          />
          <FormControl.Caption>
            {parsing
              ? strings.addParsing
              : preview
                ? `${preview.size >= 0 ? formatBytes(preview.size) : strings.addUnknownSize} · ${
                    preview.resumable ? strings.addResumable : strings.addNotResumable
                  }`
                : strings.addUrlPlaceholder}
          </FormControl.Caption>
        </FormControl>

        <div className="field-row">
          <FormControl>
            <FormControl.Label>{strings.addName}</FormControl.Label>
            <TextInput
              block
              value={name}
              placeholder={preview?.fileName ?? ""}
              onChange={(event) => setName(event.target.value)}
            />
          </FormControl>
          <FormControl>
            <FormControl.Label>{strings.addCategory}</FormControl.Label>
            <Select block value={category} onChange={(event) => setCategory(event.target.value)}>
              <Select.Option value="">{strings.addCategoryAuto}</Select.Option>
              {categories.map((c) => (
                <Select.Option key={c.id} value={c.id}>
                  {c.label}
                </Select.Option>
              ))}
            </Select>
          </FormControl>
        </div>

        <FormControl>
          <FormControl.Label>{strings.addDir}</FormControl.Label>
          <div className="field-with-button">
            <TextInput block value={dir} onChange={(event) => setDir(event.target.value)} />
            <Button
              leadingVisual={FileDirectoryIcon}
              onClick={async () => {
                const chosen = await SettingsAPI.chooseDir();
                if (chosen) setDir(chosen);
              }}
            >
              {strings.addChoose}
            </Button>
          </div>
        </FormControl>

        <FormControl>
          <FormControl.Label>
            {strings.addConnections}
            <span className="num"> · {connections}</span>
          </FormControl.Label>
          <input
            type="range"
            min={1}
            max={32}
            value={connections}
            aria-label={strings.addConnections}
            onChange={(event) => setConnections(Number(event.target.value))}
          />
        </FormControl>

        <button type="button" className="advanced-toggle" onClick={() => setAdvanced((v) => !v)} aria-expanded={advanced}>
          <ChevronDownIcon size={14} className={advanced ? "is-open" : ""} /> {strings.addAdvanced}
        </button>
        {advanced && (
          <>
            <FormControl>
              <FormControl.Label>{strings.addReferer}</FormControl.Label>
              <TextInput block value={referer} onChange={(event) => setReferer(event.target.value)} />
            </FormControl>
            <FormControl>
              <FormControl.Label>{strings.addUserAgent}</FormControl.Label>
              <TextInput block value={userAgent} onChange={(event) => setUserAgent(event.target.value)} />
            </FormControl>
            <FormControl>
              <FormControl.Label>{strings.addCookies}</FormControl.Label>
              <Textarea block rows={2} value={cookies} onChange={(event) => setCookies(event.target.value)} />
            </FormControl>
          </>
        )}
      </div>
    </Dialog>
  );
}
