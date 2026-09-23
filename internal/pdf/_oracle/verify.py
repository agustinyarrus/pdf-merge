# Oráculo triple para un PDF combinado por pdf-merge:
#   1. qpdf (vía pikepdf): chequeo estructural estricto, sin advertencias.
#   2. pypdf: cantidad de páginas y el texto de cada una en el orden esperado.
#   3. PDFium (vía pypdfium2, el motor de Chrome): cada página combinada se
#      renderiza y se compara píxel a píxel contra su página de origen.
#
# Uso: python verify.py salida.pdf origen1.pdf:1,2,3 origen2.pdf:2,3,4 ...
#   (cada origen con la lista de páginas 1-basadas que se esperan, en orden)
import sys
import numpy as np
import pikepdf
import pypdf
import pypdfium2 as pdfium

GREEN, RED, DIM, RESET = "\x1b[38;2;181;223;168m", "\x1b[38;2;242;167;184m", "\x1b[38;2;138;143;168m", "\x1b[0m"

def ok(msg):  print(f"  {GREEN}✓{RESET} {msg}")
def bad(msg): print(f"  {RED}✗{RESET} {msg}")

def main():
    out = sys.argv[1]
    expected = []  # lista de (archivo_origen, pagina_1_basada)
    for arg in sys.argv[2:]:
        path, pages = arg.rsplit(":", 1)
        expected += [(path, int(p)) for p in pages.split(",")]
    sys.exit(0 if verify(out, expected) == 0 else 1)

def verify(out, expected, render_scale=1.5):
    """Corre los tres oráculos y devuelve la cantidad de fallas."""
    fails = 0

    # 1. qpdf: chequeo de sintaxis estricto + las advertencias que junta al
    #    abrir. Solo cuentan las advertencias NUEVAS: las que ya traía algún
    #    origen se heredan fielmente y no son un defecto de la combinación.
    heredadas = set()
    for src in {s for s, _ in expected}:
        heredadas |= kinds(src)
    nuevas = kinds(out) - heredadas
    with pikepdf.open(out) as pdf:
        npages = len(pdf.pages)
    if nuevas:
        fails += 1
        bad(f"qpdf encontró {len(nuevas)} advertencias nuevas: {sorted(nuevas)[:3]}")
    else:
        extra = f", {len(heredadas)} heredadas del origen" if heredadas else ""
        ok(f"qpdf: estructura válida ({npages} páginas, sin advertencias nuevas{extra})")

    # 2. pypdf: páginas y texto
    reader = pypdf.PdfReader(out)
    if len(reader.pages) != len(expected):
        fails += 1
        bad(f"pypdf: {len(reader.pages)} páginas, se esperaban {len(expected)}")
    else:
        ok(f"pypdf: {len(reader.pages)} páginas")
    text_ok = skipped = 0
    readers = {}
    for i, (src, pno) in enumerate(expected):
        if src not in readers:
            try:
                readers[src] = pypdf.PdfReader(src)
                len(readers[src].pages)
            except Exception:
                readers[src] = None  # pypdf no lee este origen (PDFium sí lo verifica)
        if readers[src] is None:
            skipped += 1
            continue
        want = readers[src].pages[pno - 1].extract_text().strip()
        got = reader.pages[i].extract_text().strip() if i < len(reader.pages) else None
        if got == want:
            text_ok += 1
        else:
            fails += 1
            bad(f"texto de la página {i+1}: {got!r:.60} ≠ {want!r:.60}")
    if text_ok + skipped == len(expected):
        extra = f" ({skipped} de orígenes que pypdf no puede leer; los cubre PDFium)" if skipped else ""
        ok(f"pypdf: el texto de {text_ok} páginas coincide con su origen y está en orden{extra}")

    # 3. PDFium: render píxel a píxel
    merged = pdfium.PdfDocument(out)
    cache = {}
    identical = 0
    for i, (src, pno) in enumerate(expected):
        if src not in cache:
            cache[src] = pdfium.PdfDocument(src)
        a = merged[i].render(scale=render_scale).to_numpy()
        b = cache[src][pno - 1].render(scale=render_scale).to_numpy()
        if a.shape == b.shape and np.array_equal(a, b):
            identical += 1
        else:
            fails += 1
            diff = "tamaño distinto" if a.shape != b.shape else f"{int((a != b).any(axis=2).sum())} píxeles distintos"
            bad(f"render de la página {i+1} ({src}:{pno}): {diff}")
    if identical == len(expected):
        ok(f"PDFium: las {identical} páginas renderizan idénticas a su origen (0 píxeles distintos)")

    print()
    print(f"  {DIM}resultado:{RESET} {GREEN + 'todo OK' + RESET if fails == 0 else RED + str(fails) + ' fallas' + RESET}")
    return fails

def kinds(path):
    """Tipos de advertencia de qpdf, sin la ubicación (ruta, offset, objeto)."""
    out = set()
    with pikepdf.open(path) as pdf:
        for w in list(pdf.check_pdf_syntax()) + list(pdf.get_warnings()):
            s = str(w)
            out.add(s.split("): ", 1)[1] if "): " in s else s)
    return out

if __name__ == "__main__":
    main()
