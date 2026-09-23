# Estrés: combina TODOS los PDF de una lista en un único archivo con pdf-merge y
# verifica el resultado página por página con los tres oráculos.
# Uso: python bigmerge.py <pdf-merge.exe> <lista.txt> <salida.pdf>
import os, subprocess, sys, time
import pikepdf
sys.path.insert(0, os.path.dirname(__file__))
from verify import verify

exe, listfile, out = sys.argv[1], sys.argv[2], sys.argv[3]
paths = [l.strip() for l in open(listfile, encoding="utf-8-sig") if l.strip()]
if os.path.exists(out):
    os.remove(out)

t0 = time.time()
r = subprocess.run([exe, *paths, "-o", out, "--no-color"], capture_output=True, text=True, encoding="utf-8", errors="replace")
dt = time.time() - t0
if r.returncode != 0:
    print(r.stdout[-2000:])
    sys.exit(1)

expected = []
for p in paths:
    with pikepdf.open(p) as pdf:  # qpdf: más tolerante que pypdf con orígenes raros
        n = len(pdf.pages)
    expected += [(p, k) for k in range(1, n + 1)]
size_in = sum(os.path.getsize(p) for p in paths)
print(f"  pdf-merge: {len(paths)} archivos → {len(expected)} páginas en {dt*1000:.0f} ms  "
      f"({size_in/1e6:.1f} MB → {os.path.getsize(out)/1e6:.1f} MB)")
sys.exit(0 if verify(out, expected, render_scale=0.75) == 0 else 1)
