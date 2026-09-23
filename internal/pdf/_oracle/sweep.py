# Barrido de robustez: pasa cada PDF real por pdf-merge (solo) y verifica la
# salida contra qpdf, pypdf y PDFium. Imprime una tabla y un resumen.
# Uso: python sweep.py <pdf-merge.exe> <carpeta_tmp> <lista.txt con rutas>
import os, subprocess, sys, time
import numpy as np
import pikepdf, pypdf
import pypdfium2 as pdfium

exe, tmp, listfile = sys.argv[1], sys.argv[2], sys.argv[3]
os.makedirs(tmp, exist_ok=True)
paths = [l.strip() for l in open(listfile, encoding="utf-8") if l.strip()]

G, R, Y, D, X = "\x1b[38;2;181;223;168m", "\x1b[38;2;242;167;184m", "\x1b[38;2;238;223;184m", "\x1b[38;2;138;143;168m", "\x1b[0m"
stats = {"ok": 0, "cifrado": 0, "falla_merge": 0, "falla_oraculo": 0, "origen_roto": 0}
fallas = []

def warning_kinds(path):
    """Tipos de advertencia de qpdf sin ubicación (ruta/offset/objeto cambian
    entre origen y salida). Así se comparan las del origen con las de la salida
    y solo cuentan las NUEVAS: una advertencia heredada del origen no es culpa
    de pdf-merge."""
    kinds = set()
    with pikepdf.open(path) as pdf:
        for w in list(pdf.check_pdf_syntax()) + list(pdf.get_warnings()):
            s = str(w)
            kinds.add(s.split("): ", 1)[1] if "): " in s else s)
    return kinds

def src_info(p):
    """Qué dice qpdf del ORIGEN: cifrado, abrible, cuántas páginas."""
    try:
        with pikepdf.open(p) as pdf:
            return ("ok", len(pdf.pages), pdf.is_encrypted)
    except pikepdf.PasswordError:
        return ("password", 0, True)
    except Exception as e:
        return ("roto", 0, False)

for i, p in enumerate(paths):
    name = os.path.basename(p)[:44]
    out = os.path.join(tmp, f"o{i:03d}.pdf")
    if os.path.exists(out):
        os.remove(out)
    state, npages, enc = src_info(p)
    t0 = time.time()
    r = subprocess.run([exe, p, "-o", out, "--no-color"], capture_output=True, text=True, encoding="utf-8", errors="replace")
    dt = time.time() - t0
    if r.returncode != 0:
        msg = " ".join(l.strip() for l in r.stdout.splitlines() if "✗" in l)[:90]
        if "cifrado" in msg:
            stats["cifrado"] += 1
            print(f"  {Y}●{X} {name:44} {D}cifrado{X} (qpdf: {'pide contraseña' if state=='password' else 'cifrado sin contraseña de usuario' if enc else state})")
        elif state == "roto":
            stats["origen_roto"] += 1
            print(f"  {D}●{X} {name:44} {D}origen roto también para qpdf{X}")
        else:
            stats["falla_merge"] += 1
            fallas.append((p, msg))
            print(f"  {R}✗{X} {name:44} {R}{msg}{X}")
        continue
    # Oráculo sobre la salida
    problems = []
    try:
        nuevas = warning_kinds(out) - warning_kinds(p)
        if nuevas:
            problems.append(f"qpdf (nueva): {sorted(nuevas)[0][:70]}")
        with pikepdf.open(out) as pdf:
            if len(pdf.pages) != npages:
                problems.append(f"páginas {len(pdf.pages)} ≠ {npages}")
        a_doc, b_doc = pdfium.PdfDocument(out), pdfium.PdfDocument(p)
        for k in range(min(3, npages)):
            a = a_doc[k].render(scale=0.75).to_numpy()
            b = b_doc[k].render(scale=0.75).to_numpy()
            if a.shape != b.shape or not np.array_equal(a, b):
                n = "tamaño" if a.shape != b.shape else f"{int((a != b).any(axis=2).sum())} px"
                problems.append(f"render pág {k+1}: {n}")
                break
    except Exception as e:
        problems.append(f"oráculo: {type(e).__name__}: {str(e)[:60]}")
    if problems:
        stats["falla_oraculo"] += 1
        fallas.append((p, "; ".join(problems)))
        print(f"  {R}✗{X} {name:44} {R}{'; '.join(problems)[:90]}{X}")
    else:
        stats["ok"] += 1
        print(f"  {G}✓{X} {name:44} {D}{npages} pág · {dt*1000:.0f} ms{X}")

print()
print(f"  {D}resumen:{X} {G}{stats['ok']} ok{X} · {Y}{stats['cifrado']} cifrados{X} · {R}{stats['falla_merge']} fallan al combinar · {stats['falla_oraculo']} fallan el oráculo{X} · {D}{stats['origen_roto']} rotos de origen{X}")
with open(os.path.join(tmp, "fallas.txt"), "w", encoding="utf-8") as f:
    for p, m in fallas:
        f.write(f"{p}\t{m}\n")
