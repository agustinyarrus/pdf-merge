# Genera los PDF de la demo de pdf-merge en la carpeta que se le pasa. Todo es
# de prueba, con semilla fija, y reportlab en modo invariante (sin fecha ni ID
# al azar): los mismos bytes en cada corrida.
#
#   informe.pdf              12 páginas de texto, con un marcador por capítulo
#   escaneo.pdf              16 páginas "escaneadas": una imagen JPEG de papel
#                            a 300 ppp por página, cada una distinta
#   facturas/factura-01..04  una página cada una, todas con el mismo logo (lo
#                            que pdf-merge guarda una sola vez)
#
# Uso: python generar.py <carpeta>   (pip install -r ../internal/pdf/_oracle/requirements.txt)
import io
import os
import sys

import numpy as np
from PIL import Image, ImageDraw
from reportlab import rl_config
from reportlab.lib.pagesizes import A4
from reportlab.lib.utils import ImageReader
from reportlab.pdfgen import canvas

rl_config.useA85 = 0  # las imágenes, en binario (con ASCII85 ocupan un 25 % más)
out = sys.argv[1]
os.makedirs(os.path.join(out, "facturas"), exist_ok=True)
rng = np.random.default_rng(20260928)
ancho, alto = A4


def informe(ruta):
    c = canvas.Canvas(ruta, pagesize=A4, invariant=1)
    c.setTitle("Informe de obra")
    for i in range(12):
        cap = i // 3 + 1
        if i % 3 == 0:
            c.bookmarkPage(f"cap{cap}")
            c.addOutlineEntry(f"Capítulo {cap}", f"cap{cap}", level=0)
        c.setFont("Helvetica-Bold", 20)
        c.drawString(72, alto - 90, f"Informe de obra · capítulo {cap}")
        c.setFont("Helvetica", 11)
        for k in range(38):
            c.drawString(72, alto - 130 - k * 17, f"Página {i + 1} de 12, renglón {k + 1}: texto de prueba de la demo de pdf-merge.")
        c.showPage()
    c.save()


def escaneo(ruta, paginas):
    c = canvas.Canvas(ruta, pagesize=A4, invariant=1)
    c.setTitle("Escaneo")
    w, h = 2480, 3508  # A4 a 300 ppp
    for p in range(paginas):
        papel = rng.normal(236, 14, size=(h, w)).clip(0, 255).astype(np.uint8)
        img = Image.fromarray(papel, "L")
        d = ImageDraw.Draw(img)
        for k in range(60):  # renglones de "texto" escaneado
            y = 300 + k * 50
            largo = int(rng.integers(900, 2000))
            d.rectangle([240, y, 240 + largo, y + 14], fill=int(rng.integers(40, 90)))
        buf = io.BytesIO()
        img.save(buf, "JPEG", quality=85)
        buf.seek(0)
        c.drawImage(ImageReader(buf), 0, 0, width=ancho, height=alto)
        c.showPage()
    c.save()


def logo():
    img = Image.fromarray(rng.integers(0, 256, size=(600, 600, 3), dtype=np.uint8), "RGB")
    d = ImageDraw.Draw(img)
    d.ellipse([60, 60, 540, 540], fill=(196, 181, 253))
    buf = io.BytesIO()
    img.save(buf, "JPEG", quality=92)
    return buf.getvalue()


def factura(ruta, n, logo_bytes):
    c = canvas.Canvas(ruta, pagesize=A4, invariant=1)
    c.setTitle(f"Factura {n}")
    c.drawImage(ImageReader(io.BytesIO(logo_bytes)), 72, alto - 170, width=100, height=100)
    c.setFont("Helvetica-Bold", 18)
    c.drawString(200, alto - 110, f"Factura 0001-{n:08d}")
    c.setFont("Helvetica", 11)
    total = 0
    for k in range(12):
        importe = (n * 3701 + k * 1123) % 90000 / 100 + 100
        total += importe
        c.drawString(72, alto - 230 - k * 20, f"Ítem {k + 1:2d}  ...........................................  $ {importe:10.2f}")
    c.setFont("Helvetica-Bold", 12)
    c.drawString(72, alto - 500, f"Total  $ {total:.2f}")
    c.showPage()
    c.save()


informe(os.path.join(out, "informe.pdf"))
escaneo(os.path.join(out, "escaneo.pdf"), 16)
lb = logo()
for n in range(1, 5):
    factura(os.path.join(out, "facturas", f"factura-{n:02d}.pdf"), n, lb)
