import { writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";

// The Go binary embeds the build directory, and `go:embed` fails on a missing
// one. emptyOutDir wipes the directory on every build, so put the marker back.
function keepEmbedDirectory(outDir: string): Plugin {
  return {
    name: "tlc-keep-embed-directory",
    closeBundle() {
      writeFileSync(resolve(import.meta.dirname, outDir, ".gitkeep"), "");
    },
  };
}

// The build lands in internal/webui/dist so the Go binary can embed it.
// Hashed output goes under assets/build/ to keep it clear of public/assets,
// which holds the brand art the design references by name.
export default defineConfig({
  plugins: [react(), keepEmbedDirectory("../internal/webui/dist")],
  build: {
    outDir: "../internal/webui/dist",
    emptyOutDir: true,
    assetsDir: "assets/build",
    sourcemap: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.TLC_API ?? "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});
