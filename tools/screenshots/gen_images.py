"""Generate abstract card art for screenshot fixtures.

Each image is built from the Openmind warm palette so the seeded `palette`
column genuinely matches the picture — the card's palette dots are the real
dominant colours, not invented ones. Deterministic: a fixed seed per slug so
re-running produces identical files.
"""

import math
import random
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter

OUT = Path(__file__).parent / "img"
OUT.mkdir(exist_ok=True)

W, H = 1200, 800

# (slug, [hex colours]) — first colour dominates the composition.
SPECS = [
    ("cathedral", ["#1B3FD1", "#17206B", "#E4DDCD", "#C24A2E"]),
    ("kiln", ["#C24A2E", "#E0B23A", "#F4F0E6", "#1C1A16"]),
    ("moss", ["#2E7D5B", "#1F5A41", "#E4DDCD", "#E0B23A"]),
    ("brass", ["#E0B23A", "#C24A2E", "#1C1A16", "#F4F0E6"]),
    ("dusk", ["#17206B", "#1B3FD1", "#C24A2E", "#EBE5D7"]),
    ("linen", ["#E4DDCD", "#F4F0E6", "#A39C8B", "#57534A"]),
    ("ember", ["#C24A2E", "#1C1A16", "#E0B23A", "#F4F0E6"]),
    ("harbour", ["#1B3FD1", "#2E7D5B", "#F4F0E6", "#E0B23A"]),
    ("clay", ["#A39C8B", "#C24A2E", "#EBE5D7", "#1C1A16"]),
    ("indigo", ["#17206B", "#1C1A16", "#1B3FD1", "#E4DDCD"]),
    ("saffron", ["#E0B23A", "#F4F0E6", "#C24A2E", "#2E7D5B"]),
    ("slate", ["#57534A", "#A39C8B", "#E4DDCD", "#1B3FD1"]),
]


def rgb(h):
    h = h.lstrip("#")
    return tuple(int(h[i : i + 2], 16) for i in (0, 2, 4))


def gradient(size, top, bottom):
    """Vertical linear gradient, built as a 1px column then stretched."""
    w, h = size
    strip = Image.new("RGB", (1, h))
    px = strip.load()
    for y in range(h):
        t = y / max(1, h - 1)
        px[0, y] = tuple(round(top[i] + (bottom[i] - top[i]) * t) for i in range(3))
    return strip.resize((w, h), Image.BILINEAR)


def build(slug, colours, style):
    seed = sum(ord(c) for c in slug)
    rnd = random.Random(seed)
    cols = [rgb(c) for c in colours]
    img = gradient((W, H), cols[0], cols[1])
    layer = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)

    if style == 0:
        # Concentric arcs — an aperture / radiating rings.
        cx, cy = W * rnd.uniform(0.3, 0.7), H * rnd.uniform(0.35, 0.65)
        for i in range(9):
            r = 70 + i * rnd.randint(48, 72)
            c = cols[2] if i % 2 else cols[3]
            d.ellipse(
                [cx - r, cy - r, cx + r, cy + r],
                outline=c + (150 - i * 12,),
                width=rnd.randint(6, 16),
            )
    elif style == 1:
        # Stacked bands — strata, echoing the mark's three lines.
        y = -40
        while y < H:
            th = rnd.randint(26, 96)
            c = cols[rnd.randint(2, 3)]
            d.rectangle([-20, y, W + 20, y + th], fill=c + (rnd.randint(55, 135),))
            y += th + rnd.randint(30, 90)
    else:
        # Overlapping translucent discs and one hard diagonal.
        for _ in range(7):
            r = rnd.randint(120, 340)
            cx, cy = rnd.randint(0, W), rnd.randint(0, H)
            c = cols[rnd.randint(1, 3)]
            d.ellipse([cx - r, cy - r, cx + r, cy + r], fill=c + (rnd.randint(45, 105),))
        ang = rnd.uniform(-0.5, 0.5)
        y0 = H * rnd.uniform(0.3, 0.7)
        d.polygon(
            [
                (0, y0),
                (W, y0 + math.tan(ang) * W),
                (W, y0 + math.tan(ang) * W + 26),
                (0, y0 + 26),
            ],
            fill=cols[3] + (190,),
        )

    layer = layer.filter(ImageFilter.GaussianBlur(rnd.choice([0, 0, 1.2])))
    img = Image.alpha_composite(img.convert("RGBA"), layer).convert("RGB")

    # Faint paper grain so flat gradients don't look synthetic.
    grain = Image.new("L", (W // 3, H // 3))
    gp = grain.load()
    for gy in range(grain.height):
        for gx in range(grain.width):
            gp[gx, gy] = 128 + rnd.randint(-11, 11)
    grain = grain.resize((W, H), Image.BILINEAR)
    img = Image.blend(img, Image.merge("RGB", (grain, grain, grain)), 0.055)

    img.save(OUT / f"{slug}.jpg", quality=88, optimize=True)
    return slug


for i, (slug, colours) in enumerate(SPECS):
    build(slug, colours, i % 3)

print(f"wrote {len(SPECS)} images to {OUT}")
for p in sorted(OUT.iterdir()):
    print(" ", p.name, f"{p.stat().st_size // 1024}K")
