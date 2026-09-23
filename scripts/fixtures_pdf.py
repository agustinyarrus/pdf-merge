# Genera los PDF de prueba de la suite (los que leen internal/pdf y los oráculos):
#   a.pdf  3 páginas A4        b.pdf  2 páginas carta     c.pdf  5 páginas A4
#   d.pdf  2 páginas, una con imagen (streams binarios y recursos)
#   c_objstm.pdf  c.pdf reescrito con object streams y xref stream (PDF 1.5+)
# Uso: python fixtures_pdf.py <carpeta>
import os
import sys

import pikepdf
from PIL import Image, ImageDraw
from reportlab.lib.pagesizes import A4, letter
from reportlab.pdfgen import canvas


def pages(path, count, size, tag):
    c = canvas.Canvas(path, pagesize=size)
    for i in range(count):
        c.setFont("Helvetica", 40)
        c.drawString(80, size[1] - 120, f"{tag} pagina {i + 1}/{count}")
        c.showPage()
    c.save()


def with_image(path, folder):
    img = Image.new("RGB", (200, 120), (143, 214, 204))
    ImageDraw.Draw(img).text((10, 50), "IMG", fill=(11, 11, 15))
    tmp = os.path.join(folder, "_t.png")
    img.save(tmp)
    c = canvas.Canvas(path, pagesize=A4)
    c.drawString(80, 700, "D img")
    c.drawImage(tmp, 80, 400, width=200, height=120)
    c.showPage()
    c.drawString(80, 700, "D p2")
    c.showPage()
    c.save()
    os.remove(tmp)


def main(folder):
    os.makedirs(folder, exist_ok=True)
    pages(os.path.join(folder, "a.pdf"), 3, A4, "A")
    pages(os.path.join(folder, "b.pdf"), 2, letter, "B")
    pages(os.path.join(folder, "c.pdf"), 5, A4, "C")
    with_image(os.path.join(folder, "d.pdf"), folder)
    with pikepdf.open(os.path.join(folder, "c.pdf")) as pdf:
        pdf.save(os.path.join(folder, "c_objstm.pdf"), object_stream_mode=pikepdf.ObjectStreamMode.generate)


if __name__ == "__main__":
    main(sys.argv[1])
