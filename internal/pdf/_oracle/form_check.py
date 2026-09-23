# Verifica el formulario de un PDF combinado contra sus orígenes (con qpdf vía
# pikepdf, independiente de pdf-merge):
#   - la cantidad de campos raíz es la suma de los de cada origen;
#   - los nombres completos (FQN) son únicos;
#   - cada campo de cada origen está en la salida con su MISMO valor (con su
#     nombre original o renombrado con _2, _3…);
#   - todo widget de las páginas pertenece al árbol de /Fields.
# Uso: python form_check.py salida.pdf origen1.pdf origen2.pdf …
import hashlib, sys
import pikepdf

def canon(v, depth=0):
    """Forma comparable de un valor, sin números de objeto: los diccionarios se
    resuelven, las cadenas binarias (el PKCS#7 de una firma) se resumen con hash."""
    if depth > 4:
        return "…"
    if isinstance(v, pikepdf.Dictionary):
        return {str(k): canon(x, depth + 1) for k, x in sorted(v.items()) if str(k) not in ("/Parent", "/P")}
    if isinstance(v, pikepdf.Array):
        return [canon(x, depth + 1) for x in v]
    if isinstance(v, pikepdf.Stream):
        return "stream:" + hashlib.sha256(v.read_raw_bytes()).hexdigest()[:16]
    if isinstance(v, pikepdf.String):
        b = bytes(v)
        return "str:" + (b.hex() if len(b) <= 64 else hashlib.sha256(b).hexdigest()[:16])
    try:
        return str(v)
    except UnicodeDecodeError:  # cadena binaria que pikepdf no expone como String
        return "bin:" + hashlib.sha256(bytes(v)).hexdigest()[:16]

def fields(pdf):
    """Lista de (fqn, tipo, valor) de todos los campos terminales."""
    out = []
    af = pdf.Root.get("/AcroForm")
    if af is None:
        return out, set()
    reachable = set()
    def walk(f, prefix):
        reachable.add(f.objgen)
        t = str(f.get("/T", ""))
        name = f"{prefix}.{t}" if prefix and t else (t or prefix)
        kids = f.get("/Kids", [])
        field_kids = [k for k in kids if "/T" in k]
        for k in kids:
            if "/T" not in k:
                reachable.add(k.objgen)  # widget hijo
        if field_kids:
            for k in field_kids:
                walk(k, name)
        else:
            out.append((name, str(f.get("/FT", "")), repr(canon(f.get("/V")))))
    for f in af.get("/Fields", []):
        walk(f, "")
    return out, reachable

def same_field(merged, original):
    """¿merged es original, o original renombrado por pdf-merge (_2, _3… en el
    primer componente)? No alcanza con recortar cualquier "_N" final: hay
    formularios cuyos nombres ORIGINALES ya terminan en _0 o _1."""
    mf, _, mrest = merged.partition(".")
    of, _, orest = original.partition(".")
    if mrest != orest:
        return False
    if mf == of:
        return True
    if mf.startswith(of + "_"):
        n = mf[len(of) + 1:]
        return n.isdigit() and int(n) >= 2
    return False

def main():
    out_path, sources = sys.argv[1], sys.argv[2:]
    ok = True
    with pikepdf.open(out_path) as pdf:
        got, reachable = fields(pdf)
        roots = len(pdf.Root.AcroForm.get("/Fields", [])) if "/AcroForm" in pdf.Root else 0
        orphans = 0
        for page in pdf.pages:
            for a in page.obj.get("/Annots", []):
                if a.get("/Subtype") == "/Widget" and a.objgen not in reachable:
                    parent = a.get("/Parent")
                    if parent is None or parent.objgen not in reachable:
                        orphans += 1
    want_roots, expected = 0, []
    for s in sources:
        with pikepdf.open(s) as src:
            want_roots += len(src.Root.AcroForm.get("/Fields", [])) if "/AcroForm" in src.Root else 0
            expected += fields(src)[0]

    names = [n for n, _, _ in got]
    checks = [
        (roots == want_roots, f"campos raíz: {roots} (esperados {want_roots})"),
        (len(names) == len(set(names)), f"nombres completos únicos: {len(set(names))} de {len(names)}"),
        (orphans == 0, f"widgets sin campo: {orphans}"),
    ]
    # Primero los nombres exactos, después los renombrados: así un "Numero_1"
    # original no se lo lleva un "Numero_1_2" que corresponde a otro archivo.
    pool = list(got)
    missing = []
    pending = []
    for name, ft, val in expected:
        match = next((g for g in pool if g[0] == name and g[1] == ft and g[2] == val), None)
        if match:
            pool.remove(match)
        else:
            pending.append((name, ft, val))
    for name, ft, val in pending:
        match = next((g for g in pool if same_field(g[0], name) and g[1] == ft and g[2] == val), None)
        if match:
            pool.remove(match)
        else:
            missing.append(name)
    checks.append((not missing, f"campos con su valor original: {len(expected) - len(missing)} de {len(expected)}" + (f" (faltan {missing[:4]})" if missing else "")))
    for good, msg in checks:
        print(f"  {'✓' if good else '✗'} {msg}")
        ok &= good
    originals = {n for n, _, _ in expected}
    renamed = sorted(n for n in names if n not in originals)
    if renamed:
        print(f"  · renombrados: {', '.join(renamed[:6])}{'…' if len(renamed) > 6 else ''}")
    sys.exit(0 if ok else 1)

if __name__ == "__main__":
    main()
