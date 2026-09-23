# Diagnóstico: compara los diccionarios de fuente (Font, FontDescriptor) y los
# ExtGState del origen y la salida, emparejados por nombre, en forma canónica
# (referencias reemplazadas por su contenido resuelto, sin números de objeto).
import sys
import pikepdf

src_path, out_path = sys.argv[1], sys.argv[2]

def canon(o, depth=0):
    """Forma canónica comparable: resuelve referencias hasta 3 niveles."""
    if depth > 3:
        return "…"
    if isinstance(o, pikepdf.Stream):
        return "<stream " + str(len(o.read_raw_bytes())) + ">"
    if isinstance(o, pikepdf.Dictionary):
        return {str(k): canon(v, depth + 1) for k, v in sorted(o.items()) if str(k) not in ("/Parent",)}
    if isinstance(o, pikepdf.Array):
        return [canon(v, depth + 1) for v in o]
    return repr(o) if not isinstance(o, (int, float)) else o

def collect(pdf):
    fonts, gs = {}, {}
    for page in pdf.pages:
        res = page.obj.get("/Resources", {})
        for name, f in (res.get("/Font", {}) or {}).items():
            key = str(f.get("/BaseFont", name))
            fonts.setdefault(key, f)
        for name, g in (res.get("/ExtGState", {}) or {}).items():
            gs.setdefault(str(name), g)
    return fonts, gs

with pikepdf.open(src_path) as src, pikepdf.open(out_path) as out:
    sf, sg = collect(src)
    of, og = collect(out)
    print(f"fuentes origen={len(sf)} salida={len(of)}   extgstate origen={len(sg)} salida={len(og)}")
    for name in sorted(set(sf) | set(of)):
        a, b = sf.get(name), of.get(name)
        if a is None or b is None:
            print("  fuente solo en", "salida" if a is None else "origen", name)
            continue
        ca, cb = canon(a), canon(b)
        if ca != cb:
            print("  DIFIERE fuente", name)
            for k in sorted(set(ca) | set(cb)):
                if ca.get(k) != cb.get(k):
                    print(f"     {k}:\n        origen={str(ca.get(k))[:160]}\n        salida={str(cb.get(k))[:160]}")
    for name in sorted(set(sg) | set(og)):
        a, b = sg.get(name), og.get(name)
        if a is not None and b is not None and canon(a) != canon(b):
            print("  DIFIERE extgstate", name, "\n     origen", canon(a), "\n     salida", canon(b))
