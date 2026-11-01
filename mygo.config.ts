import { defineConfig } from "mygo-cli";

export default defineConfig({
  name: "Gale",
  slug: "gale",
  identifier: "app.gale.desktop",
  version: "0.1.0",
  devUrl: "http://127.0.0.1:5173",
  devCommand: "bun run dev:web",
  buildCommand: "bun run build:web",
  frontendDist: "dist",
  bindings: "src/mygo.ts",
  out: "build",
  windows: {
    displayName: "Gale 下载器",
    installerInclude: "installer/installer.nsh",
  },
});
