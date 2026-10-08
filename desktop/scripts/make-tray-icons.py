"""Draws the system tray icons (the Tunnelkey arch with a keyhole) in three
states and writes desktop/trayicons/{idle,busy,secure}.{ico,png}.

    python desktop/scripts/make-tray-icons.py

The shapes follow the app logo (LogoMark in the UI): a thick outer arch, a thin
inner arch and a keyhole. Drawn at 4x and downsampled for smooth edges.
"""

from pathlib import Path

from PIL import Image, ImageDraw

OUT = Path(__file__).resolve().parents[1] / "trayicons"

# Arch colour per state; inner arch and keyhole stay neutral.
STATES = {
    "idle": (147, 161, 176),    # slate grey: not connected
    "busy": (227, 169, 59),     # brass: connecting / reconnecting
    "secure": (60, 203, 155),   # teal-green: protected
}
INNER = (120, 135, 150)
KEYHOLE = (245, 243, 238)
SIZES = [16, 20, 24, 32, 40, 48, 64]


def arch(draw, box, width, colour, bottom):
    """Arch: two legs plus a half circle, open at the bottom."""
    x0, y0, x1, _ = box
    r = (x1 - x0) / 2
    cy = y0 + r
    draw.arc([x0, y0, x1, y0 + 2 * r], 180, 360, fill=colour, width=width)
    draw.line([(x0 + width / 2, cy), (x0 + width / 2, bottom)], fill=colour, width=width)
    draw.line([(x1 - width / 2, cy), (x1 - width / 2, bottom)], fill=colour, width=width)
    # round feet
    for x in (x0 + width / 2, x1 - width / 2):
        draw.ellipse([x - width / 2, bottom - width / 2, x + width / 2, bottom + width / 2], fill=colour)


def render(colour, size):
    s = size * 4
    img = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    pad = s * 0.06
    bottom = s - pad - s * 0.08
    outer_w = max(4, int(s * 0.17))
    arch(d, (pad, pad, s - pad, s), outer_w, colour, bottom)
    inner_pad = pad + outer_w + s * 0.06
    if size >= 24:  # the thin inner arch disappears at tiny sizes
        arch(d, (inner_pad, inner_pad, s - inner_pad, s), max(2, int(s * 0.05)), INNER, bottom)
    # keyhole: circle + tapered body
    cx, cy, kr = s / 2, s * 0.52, s * 0.085
    d.ellipse([cx - kr, cy - kr, cx + kr, cy + kr], fill=KEYHOLE)
    d.polygon([(cx - kr * 0.6, cy), (cx + kr * 0.6, cy), (cx + kr * 0.95, s * 0.76), (cx - kr * 0.95, s * 0.76)], fill=KEYHOLE)
    return img.resize((size, size), Image.LANCZOS)


def main():
    OUT.mkdir(exist_ok=True)
    for name, colour in STATES.items():
        images = [render(colour, sz) for sz in SIZES]
        images[-1].save(OUT / f"{name}.ico", sizes=[(sz, sz) for sz in SIZES], append_images=images[:-1])
        images[-1].save(OUT / f"{name}.png")
        print("wrote", name)


if __name__ == "__main__":
    main()
