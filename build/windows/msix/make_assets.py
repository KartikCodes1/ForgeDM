#!/usr/bin/env python3
"""Generates the MSIX logo set in build/windows/msix/Assets from build/appicon.png.

Run from the repository root:  python3 build/windows/msix/make_assets.py
Needs Pillow (pip install pillow). The generated PNGs are committed, so this
only needs re-running when the app icon changes.
"""
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parents[3]
SRC = ROOT / "build" / "appicon.png"
OUT = Path(__file__).resolve().parent / "Assets"
TILE_BG = (10, 14, 23, 255)  # ForgeDM's window background

SCALES = [100, 125, 150, 200, 400]
TARGET_SIZES = [16, 20, 24, 30, 32, 36, 40, 48, 60, 64, 72, 80, 96, 256]


def icon(size: int) -> Image.Image:
    return Image.open(SRC).convert("RGBA").resize((size, size), Image.LANCZOS)


def tile(w: int, h: int, logo_frac: float) -> Image.Image:
    """Dark tile with the logo centred, a little above middle to leave room
    for the name Windows draws at the bottom."""
    im = Image.new("RGBA", (w, h), TILE_BG)
    s = round(min(w, h) * logo_frac)
    im.alpha_composite(icon(s), ((w - s) // 2, round((h - s) * 0.42)))
    return im


def save(im: Image.Image, name: str) -> None:
    im.save(OUT / name, optimize=True)


def main() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    for f in OUT.glob("*.png"):
        f.unlink()
    for sc in SCALES:
        k = sc / 100
        # App list, taskbar and title bar icon: the logo itself.
        save(icon(round(44 * k)), f"Square44x44Logo.scale-{sc}.png")
        save(icon(round(50 * k)), f"StoreLogo.scale-{sc}.png")
        save(tile(round(71 * k), round(71 * k), 0.62), f"SmallTile.scale-{sc}.png")
        save(tile(round(150 * k), round(150 * k), 0.50), f"Square150x150Logo.scale-{sc}.png")
        save(tile(round(310 * k), round(150 * k), 0.50), f"Wide310x150Logo.scale-{sc}.png")
        save(tile(round(310 * k), round(310 * k), 0.45), f"LargeTile.scale-{sc}.png")
    for ts in TARGET_SIZES:
        im = icon(ts)
        save(im, f"Square44x44Logo.targetsize-{ts}.png")
        save(im, f"Square44x44Logo.targetsize-{ts}_altform-unplated.png")
        save(im, f"Square44x44Logo.targetsize-{ts}_altform-lightunplated.png")
    print(f"wrote {len(list(OUT.glob('*.png')))} images to {OUT}")


if __name__ == "__main__":
    main()
