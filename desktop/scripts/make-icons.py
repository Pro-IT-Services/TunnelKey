#!/usr/bin/env python3
"""Regenerate desktop icons from the shared Tunnelkey artwork.

Source: ios/.../AppIcon-1024.png (native 1024 px, same artwork as the
Android icon).

Outputs (relative to desktop/):
  build/appicon.png                              1024x1024 RGBA (Wails: macOS icns, Linux)
  build/windows/icon.ico                         16..256 multi-size
  build/linux/icons/hicolor/<N>x<N>/apps/tunnelkey.png
  build/windows/installer/logo.png               64x64 (setup.exe bootstrapper UI)

Requires Pillow (pip install pillow).
"""
import os
import sys

from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
DESKTOP = os.path.dirname(HERE)
REPO = os.path.dirname(DESKTOP)

SRC_1024 = os.path.join(REPO, "ios", "Tunnelkey", "Resources", "Assets.xcassets",
                        "AppIcon.appiconset", "AppIcon-1024.png")

ICO_SIZES = [16, 20, 24, 32, 40, 48, 64, 96, 128, 256]
LINUX_SIZES = [32, 48, 64, 128, 256, 512]


def load_master():
    im = Image.open(SRC_1024).convert("RGBA")
    src = SRC_1024
    if im.size != (1024, 1024):
        im = im.resize((1024, 1024), Image.LANCZOS)
    print("source:", os.path.relpath(src, REPO))
    return im


def main():
    master = load_master()

    out = os.path.join(DESKTOP, "build", "appicon.png")
    master.save(out, optimize=True)
    print("wrote", os.path.relpath(out, REPO), master.size)

    ico = os.path.join(DESKTOP, "build", "windows", "icon.ico")
    master.save(ico, format="ICO", sizes=[(s, s) for s in ICO_SIZES])
    print("wrote", os.path.relpath(ico, REPO), ICO_SIZES)

    logo = os.path.join(DESKTOP, "build", "windows", "installer", "logo.png")
    master.resize((64, 64), Image.LANCZOS).save(logo, optimize=True)
    print("wrote", os.path.relpath(logo, REPO))

    for s in LINUX_SIZES:
        d = os.path.join(DESKTOP, "build", "linux", "icons", "hicolor", f"{s}x{s}", "apps")
        os.makedirs(d, exist_ok=True)
        p = os.path.join(d, "tunnelkey.png")
        master.resize((s, s), Image.LANCZOS).save(p, optimize=True)
        print("wrote", os.path.relpath(p, REPO))
    return 0


if __name__ == "__main__":
    sys.exit(main())
