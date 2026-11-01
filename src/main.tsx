import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { TakeoverPage } from "./pages/TakeoverPage";

// Primer's design tokens: spacing, type and radii from primitives.css, colors
// from the theme files, which ThemeProvider selects through data attributes.
import "@primer/primitives/dist/css/primitives.css";
import "@primer/primitives/dist/css/functional/themes/light.css";
import "@primer/primitives/dist/css/functional/themes/light-high-contrast.css";
import "@primer/primitives/dist/css/functional/themes/dark.css";
import "@primer/primitives/dist/css/functional/themes/dark-high-contrast.css";
import "./styles.css";

// The takeover window is the same page with the download it shows in its
// query, so both windows share one bundle and one dev server.
const takeoverParam = new URLSearchParams(window.location.search).get("takeover");
const takeoverId = takeoverParam === null ? null : Number(takeoverParam);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    {takeoverId !== null && Number.isSafeInteger(takeoverId) ? <TakeoverPage id={takeoverId} /> : <App />}
  </StrictMode>,
);
