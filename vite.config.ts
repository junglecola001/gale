import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // The bundle is embedded in the app and read from disk, never downloaded,
  // so its size is not a budget; the default warning is noise here.
  build: { chunkSizeWarningLimit: 1200 },
  server: {
    // The app loads this address during `mygo dev` (devUrl in mygo.config.ts).
    // IPv4 explicitly: on Windows `localhost` may resolve to ::1, which the
    // webview and this machine's firewall do not always reach.
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    // The development app and the packaged builds are not the page's.
    watch: { ignored: ["**/.mygo/**", "**/build/**"] },
  },
});
