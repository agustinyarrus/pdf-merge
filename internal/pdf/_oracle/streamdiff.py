# Diagnóstico: qué streams de la salida NO existen byte a byte en el origen, y
# qué claves de página difieren. Uso: python streamdiff.py origen.pdf salida.pdf
import hashlib, sys
import pikepdf

src_path, out_path = sys.argv[1], sys.argv[2]

def stream_index(pdf):
    idx = {}
    for obj in pdf.objects:
        if isinstance(obj, pikepdf.Stream):
            raw = obj.read_raw_bytes()
            idx.setdefault(hashlib.sha256(raw).hexdigest(), []).append(obj)
    return idx

with pikepdf.open(src_path) as src, pikepdf.open(out_path) as out:
    s_idx = stream_index(src)
    missing = []
    total = 0
    for obj in out.objects:
        if isinstance(obj, pikepdf.Stream):
            total += 1
            raw = obj.read_raw_bytes()
            if hashlib.sha256(raw).hexdigest() not in s_idx:
                missing.append((obj.objgen, len(raw), dict((k, str(v)[:40]) for k, v in obj.stream_dict.items())))
    print(f"streams en salida: {total}   sin gemelo exacto en el origen: {len(missing)}")
    for og, n, d in missing[:6]:
        print(f"  obj {og}  {n} bytes  {d}")

    # Claves de la primera página: origen vs salida
    sp, op = src.pages[0].obj, out.pages[0].obj
    sk, ok = set(sp.keys()), set(op.keys())
    print("claves pág 1 solo en origen:", sorted(sk - ok))
    print("claves pág 1 solo en salida:", sorted(ok - sk))
    for k in sorted(sk & ok):
        if k in ("/Parent", "/Contents", "/Resources", "/Annots"):
            continue
        if str(sp[k]) != str(op[k]):
            print(f"  {k}: origen={str(sp[k])[:60]}  salida={str(op[k])[:60]}")
    print("catálogo origen:", sorted(src.Root.keys()))
    print("annots pág 1 origen:", len(sp.get("/Annots", [])), " salida:", len(op.get("/Annots", [])))
    res_s, res_o = sp.get("/Resources"), op.get("/Resources")
    if res_s is not None and res_o is not None:
        print("recursos origen:", sorted(res_s.keys()), " salida:", sorted(res_o.keys()))
