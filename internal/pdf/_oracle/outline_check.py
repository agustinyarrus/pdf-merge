# Genera un PDF con marcadores anidados y enlaces internos por NOMBRE, y verifica
# (con qpdf vía pikepdf, independiente de pdf-merge) los marcadores y enlaces de
# una salida combinada.
#
#   python outline_check.py gen <carpeta>
#   python outline_check.py check <salida.pdf> <esperado.json>
import json, os, sys
import pikepdf

def gen(folder):
    from reportlab.pdfgen import canvas
    from reportlab.lib.pagesizes import A4
    path = os.path.join(folder, "links.pdf")
    c = canvas.Canvas(path, pagesize=A4)
    outline = {1: ("Capítulo 1", 0), 3: ("Capítulo 2 · diseño", 0), 4: ("Sección 2.1 — ñandú", 1), 5: ("Capítulo 3", 0)}
    links = {1: 3, 2: 5, 3: 5, 4: 1}  # página → destino (por nombre)
    for p in range(1, 6):
        c.bookmarkPage(f"p{p}")
        if p in outline:
            title, level = outline[p]
            c.addOutlineEntry(title, f"p{p}", level=level)
        c.setFont("Helvetica", 28)
        c.drawString(72, 760, f"links.pdf pagina {p}")
        if p in links:
            c.drawString(72, 700, f"-> ir a {links[p]}")
            c.linkAbsolute("", f"p{links[p]}", Rect=(70, 690, 300, 730))
        c.showPage()
    c.save()
    print("generado", path)
    gen_named(path, os.path.join(folder, "links_named.pdf"))

def gen_named(src, dst):
    """Reescribe todos los destinos de src en las formas CON NOMBRE que existen,
    para ejercitar la resolución: /Dest cadena, /A GoTo /D cadena, /Dest Name
    contra el /Dests de PDF 1.1, y un árbol /Names /Dests con nodos /Kids."""
    with pikepdf.open(src) as pdf:
        pages = pdf.pages
        explicit = {f"cap{i+1}": pikepdf.Array([pages[i].obj, pikepdf.Name("/XYZ"), None, 800, None]) for i in range(len(pages))}
        # Árbol de nombres de dos hojas bajo un nodo raíz con /Kids (claves ordenadas).
        keys = sorted(explicit)
        mid = len(keys) // 2
        def leaf(ks):
            arr = pikepdf.Array()
            for k in ks:
                arr.append(pikepdf.String(k)); arr.append(explicit[k])
            return pdf.make_indirect(pikepdf.Dictionary(Names=arr, Limits=pikepdf.Array([pikepdf.String(ks[0]), pikepdf.String(ks[-1])])))
        tree = pdf.make_indirect(pikepdf.Dictionary(Kids=pikepdf.Array([leaf(keys[:mid]), leaf(keys[mid:])])))
        pdf.Root.Names = pikepdf.Dictionary(Dests=tree)
        # Estilo PDF 1.1: /Dests con claves Name (solo para "old3").
        pdf.Root.Dests = pikepdf.Dictionary({"/old3": pikepdf.Dictionary(D=explicit["cap3"])})

        def name_for(dest):
            target = dest[0]
            for i, p in enumerate(pages):
                if p.obj.objgen == target.objgen:
                    return f"cap{i+1}"
        # El primer enlace (página 1 → 3) usa el Name de PDF 1.1; el segundo la
        # acción GoTo; los demás /Dest cadena: las tres formas quedan cubiertas.
        forms = ["name11", "goto", "string", "string"]
        n = 0
        for page in pages:
            for a in page.obj.get("/Annots", []):
                if a.get("/Subtype") != "/Link":
                    continue
                d = a.get("/Dest") if "/Dest" in a else a.A.D
                key = name_for(d)
                form = forms[n % len(forms)]
                n += 1
                if "/Dest" in a: del a["/Dest"]
                if "/A" in a: del a["/A"]
                if form == "string":
                    a.Dest = pikepdf.String(key)
                elif form == "goto":
                    a.A = pikepdf.Dictionary(S=pikepdf.Name("/GoTo"), D=pikepdf.String(key))
                elif key == "cap3":
                    a.Dest = pikepdf.Name("/old3")
                else:
                    a.Dest = pikepdf.String(key)
        # Marcadores: uno por /A GoTo con cadena, el resto /Dest cadena.
        with pdf.open_outline() as ol:
            def rec(items, depth):
                for j, it in enumerate(items):
                    d = it.destination
                    if d is not None and not isinstance(d, (pikepdf.String, pikepdf.Name)):
                        key = name_for(d)
                        if depth == 1:
                            it.destination = None
                            it.action = pikepdf.Dictionary(S=pikepdf.Name("/GoTo"), D=pikepdf.String(key))
                        else:
                            it.destination = pikepdf.String(key)
                    rec(it.children, depth + 1)
            rec(ol.root, 0)
        pdf.save(dst)
    print("generado", dst)

def page_index(pdf, dest):
    """Índice 1-basado de la página a la que apunta un destino (array explícito o nombre)."""
    if dest is None:
        return None
    if isinstance(dest, (pikepdf.String, pikepdf.Name, str)):
        return ("nombre", str(dest))
    target = dest[0]
    for i, p in enumerate(pdf.pages):
        if p.obj.objgen == target.objgen:
            return i + 1
    return ("huérfano", repr(target)[:40])

def walk_outline(pdf):
    out = []
    with pdf.open_outline() as ol:
        def rec(items, depth):
            for it in items:
                dest = it.destination
                if dest is None and it.action is not None and it.action.get("/S") == "/GoTo":
                    dest = it.action.get("/D")
                out.append({"titulo": it.title, "nivel": depth, "pagina": page_index(pdf, dest)})
                rec(it.children, depth + 1)
        rec(ol.root, 0)
    return out

def links(pdf):
    out = []
    for i, p in enumerate(pdf.pages):
        for a in p.obj.get("/Annots", []):
            if a.get("/Subtype") != "/Link":
                continue
            d = a.get("/Dest")
            if d is None and "/A" in a and a.A.get("/S") == "/GoTo":
                d = a.A.get("/D")
            out.append({"desde": i + 1, "hacia": page_index(pdf, d) if d is not None else "inerte"})
    return out

def check(out_path, expected_path):
    exp = json.load(open(expected_path, encoding="utf-8"))
    with pikepdf.open(out_path) as pdf:
        got = {"marcadores": walk_outline(pdf), "enlaces": links(pdf),
               "pagemode": str(pdf.Root.get("/PageMode", ""))}
    ok = True
    for key in ("marcadores", "enlaces", "pagemode"):
        if got[key] == exp[key]:
            print(f"  ✓ {key}: {len(got[key]) if isinstance(got[key], list) else got[key]} como se esperaba")
        else:
            ok = False
            print(f"  ✗ {key}:\n      obtenido {got[key]}\n      esperado {exp[key]}")
    sys.exit(0 if ok else 1)

if __name__ == "__main__":
    {"gen": lambda: gen(sys.argv[2]), "check": lambda: check(sys.argv[2], sys.argv[3])}[sys.argv[1]]()
