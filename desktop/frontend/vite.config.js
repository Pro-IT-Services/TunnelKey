import { writeFileSync } from "node:fs";
import { defineConfig } from "vite";

export default defineConfig({
  build: { target: "es2022" },
  esbuild: { target: "es2022" },
  plugins: [
    {
      // dist/ is emptied on every build, but `//go:embed all:frontend/dist`
      // needs it to exist in a clean checkout, so keep the tracked placeholder.
      name: "keep-dist-placeholder",
      closeBundle() {
        writeFileSync(new URL("./dist/gitkeep", import.meta.url), "");
      },
    },
  ],
});
