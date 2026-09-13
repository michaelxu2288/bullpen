import { writeFileSync } from "node:fs";
import { resolve } from "node:path";

import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";

const OUT_DIR = "../internal/web/dist";

/**
 * `//go:embed all:dist` needs the directory to contain at least one file or the
 * Go build fails on a fresh clone, where the bundle has never been built. Vite's
 * emptyOutDir removes the committed placeholder, so put it back afterwards.
 */
function keepEmbedDirPopulated(): Plugin {
  return {
    name: "bullpen-keep-embed-dir",
    closeBundle() {
      writeFileSync(resolve(__dirname, OUT_DIR, ".gitkeep"), "");
    },
  };
}

// The bundle is embedded by internal/web, so it builds straight into the Go tree.
// Dev mode proxies the API to a `bullpen server` running on 7070.
export default defineConfig({
  plugins: [react(), keepEmbedDirPopulated()],
  build: {
    outDir: OUT_DIR,
    emptyOutDir: true,
    // one file each keeps the go:embed listing readable
    assetsDir: "assets",
  },
  server: {
    port: 5173,
    proxy: {
      "/v1": {
        target: "http://127.0.0.1:7070",
        changeOrigin: true,
      },
    },
  },
});
