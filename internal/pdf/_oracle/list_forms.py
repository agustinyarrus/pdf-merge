# Imprime, de una lista de PDF (uno por línea), los que traen formulario
# (/AcroForm). Sirve para elegir el corpus de la prueba de formularios por
# CONTENIDO, sin depender de cómo se llamen los archivos.
# Uso: python list_forms.py lista.txt
import sys
import pikepdf

for line in open(sys.argv[1], encoding="utf-8-sig"):
    path = line.strip()
    if not path:
        continue
    try:
        with pikepdf.open(path) as pdf:
            if "/AcroForm" in pdf.Root and len(pdf.Root.AcroForm.get("/Fields", [])) > 0:
                print(path)
    except Exception:
        pass  # un PDF que qpdf no abre no es candidato para esta prueba
