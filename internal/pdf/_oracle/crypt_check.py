# Batería de cifrado: cifra PDF de prueba con qpdf (pikepdf) en todas las
# revisiones del manejador estándar, los pasa por pdf-merge y verifica que la
# salida (descifrada) renderice IDÉNTICA al original sin cifrar.
# Uso: python crypt_check.py <pdf-merge.exe> <carpeta de fixtures>
import os, subprocess, sys
import pikepdf
sys.path.insert(0, os.path.dirname(__file__))
from verify import verify

exe, fx = sys.argv[1], sys.argv[2]
G, R, D, X = "\x1b[38;2;181;223;168m", "\x1b[38;2;242;167;184m", "\x1b[38;2;138;143;168m", "\x1b[0m"
restrict = pikepdf.Permissions(print_highres=False, extract=False, modify_other=False)

# (nombre, original, parámetros de cifrado, contraseñas para pdf-merge, objstm)
cases = [
    # En R2/R3 no existe /EncryptMetadata: qpdf exige metadata=False.
    ("R2 RC4-40, sin contraseña de apertura",   "d.pdf",          dict(R=2, aes=False, metadata=False), [], False),
    ("R3 RC4-128, sin contraseña de apertura",  "d.pdf",          dict(R=3, aes=False, metadata=False), [], False),
    ("R4 RC4-128 (EncryptMetadata false)",      "links_named.pdf", dict(R=4, aes=False, metadata=False), [], False),
    ("R4 AES-128",                              "links_named.pdf", dict(R=4, aes=True), [], False),
    ("R6 AES-256",                              "d.pdf",          dict(R=6, aes=True), [], False),
    ("R6 AES-256, EncryptMetadata false",       "d.pdf",          dict(R=6, aes=True, metadata=False), [], False),
    ("R6 AES-256 + object streams cifrados",    "c_objstm.pdf",   dict(R=6, aes=True), [], True),
    ("R4 AES-128 + object streams cifrados",    "c_objstm.pdf",   dict(R=4, aes=True), [], True),
    ("R6 con contraseña de usuario",            "d.pdf",          dict(R=6, aes=True, user="secreta"), ["secreta"], False),
    ("R4 con contraseña de usuario",            "links_named.pdf", dict(R=4, aes=True, user="secreta"), ["secreta"], False),
    ("R3 abierto con la de PROPIETARIO",        "d.pdf",          dict(R=3, aes=False, metadata=False, user="secreta"), ["dueño123"], False),
    ("R6 abierto con la de PROPIETARIO",        "d.pdf",          dict(R=6, aes=True, user="secreta"), ["dueño123"], False),
]

fails = 0
for i, (name, orig, enc, passwords, objstm) in enumerate(cases, 1):
    src = os.path.join(fx, orig)
    encp = os.path.join(fx, f"enc_{i:02d}.pdf")
    out = os.path.join(fx, f"enc_{i:02d}_out.pdf")
    user = enc.pop("user", "")
    with pikepdf.open(src) as pdf:
        kw = {"object_stream_mode": pikepdf.ObjectStreamMode.generate} if objstm else {}
        pdf.save(encp, encryption=pikepdf.Encryption(owner="dueño123", user=user, allow=restrict, **enc), **kw)
    if os.path.exists(out):
        os.remove(out)
    args = [exe, encp, "-o", out, "--no-color"]
    for p in passwords:
        args += ["--password", p]
    r = subprocess.run(args, capture_output=True, text=True, encoding="utf-8", errors="replace")
    print(f"{D}caso {i:2d}{X} {name}")
    if r.returncode != 0:
        fails += 1
        print(f"  {R}✗ pdf-merge salió con {r.returncode}:{X} {[l.strip() for l in r.stdout.splitlines() if '✗' in l][:1]}")
        continue
    with pikepdf.open(src) as p:
        n = len(p.pages)
    if verify(out, [(src, k) for k in range(1, n + 1)], render_scale=1.0) != 0:
        fails += 1
    if "restricciones" not in r.stdout:
        fails += 1
        print(f"  {R}✗ faltó el aviso de restricciones del propietario{X}")

# Negativo: con contraseña de usuario y SIN --password tiene que fallar claro.
print(f"{D}caso {len(cases)+1:2d}{X} contraseña de usuario sin --password (tiene que fallar)")
r = subprocess.run([exe, os.path.join(fx, "enc_09.pdf"), "-o", os.path.join(fx, "enc_neg.pdf"), "--no-color", "--force"],
                   capture_output=True, text=True, encoding="utf-8", errors="replace")
if r.returncode != 0 and "pide contraseña" in r.stdout:
    print(f"  {G}✓{X} falla con \"pide contraseña\" (exit {r.returncode})")
else:
    fails += 1
    print(f"  {R}✗ esperaba fallo claro; exit {r.returncode}{X}")

print()
print(f"  {D}resultado:{X} " + (f"{G}los {len(cases)+1} casos OK{X}" if fails == 0 else f"{R}{fails} fallas{X}"))
sys.exit(0 if fails == 0 else 1)
