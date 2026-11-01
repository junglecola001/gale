import { useEffect, useState } from "react";
import { IconButton, TextInput } from "@primer/react";
import { DashIcon, GlobeIcon, SearchIcon, ScreenFullIcon, ScreenNormalIcon, XIcon } from "@primer/octicons-react";
import { currentWindow, isMyGo } from "mygo-runtime";
import { useApp } from "../store/useApp";
import { strings } from "../lib/strings";

/** TitleBar draws the window's own title bar: the name, the search field,
 * the browser state and the window buttons. */
export function TitleBar() {
  const search = useApp((s) => s.search);
  const setSearch = useApp((s) => s.setSearch);
  const browser = useApp((s) => s.browser);
  const openSettings = useApp((s) => s.openSettings);
  const [maximized, setMaximized] = useState(false);

  useEffect(() => {
    if (!isMyGo()) return;
    let alive = true;
    const read = () => {
      currentWindow.isMaximized().then((value) => {
        if (alive) setMaximized(value);
      });
    };
    read();
    window.addEventListener("resize", read);
    return () => {
      alive = false;
      window.removeEventListener("resize", read);
    };
  }, []);

  const connected = Boolean(browser?.connected);
  return (
    <header
      className="tb"
      onDoubleClick={() => {
        if (isMyGo()) void currentWindow.toggleMaximize();
      }}
    >
      <div className="tb-brand">
        <span className="tb-logo" aria-hidden>
          ◈
        </span>
        <span className="tb-name">{strings.appName}</span>
      </div>

      <div className="tb-search" onDoubleClick={(event) => event.stopPropagation()}>
        <TextInput
          id="gale-search"
          size="small"
          block
          leadingVisual={SearchIcon}
          placeholder={strings.search}
          aria-label={strings.search}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          trailingAction={
            search ? (
              <IconButton
                icon={XIcon}
                aria-label="清除搜索"
                variant="invisible"
                size="small"
                onClick={() => setSearch("")}
              />
            ) : undefined
          }
        />
      </div>

      <div className="tb-right">
        <button
          type="button"
          className={`tb-browser${connected ? " is-on" : ""}`}
          title={connected ? strings.connected : strings.disconnected}
          aria-label={strings.browser}
          onClick={() => openSettings("browser")}
        >
          <GlobeIcon size={14} />
          <span className="tb-dot" aria-hidden />
        </button>
        <div className="tb-controls" onDoubleClick={(event) => event.stopPropagation()}>
          <button type="button" aria-label="最小化" onClick={() => isMyGo() && void currentWindow.minimize()}>
            <DashIcon size={14} />
          </button>
          <button
            type="button"
            aria-label={maximized ? "还原" : "最大化"}
            onClick={() => isMyGo() && void currentWindow.toggleMaximize()}
          >
            {maximized ? <ScreenNormalIcon size={14} /> : <ScreenFullIcon size={14} />}
          </button>
          <button type="button" className="tb-close" aria-label="关闭" onClick={() => isMyGo() && void currentWindow.close()}>
            <XIcon size={14} />
          </button>
        </div>
      </div>
    </header>
  );
}
