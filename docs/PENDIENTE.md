# Pendiente

pdf-merge está terminada y verificada ([VERIFICACION.md](VERIFICACION.md)). Lo que sigue son mejoras, en el orden en que conviene hacerlas.

## Salida

- **Object streams y xref stream en la salida**: hoy se escribe una tabla xref clásica, que es la forma más compatible pero no la más chica. Con object streams la estructura ocuparía bastante menos en documentos de muchas páginas.
- **Recomprimir los streams que vienen sin filtro** (algunos generadores escriben el contenido de las páginas en claro).

## Lo que hoy se pierde al combinar

- **Numeración de páginas propia** de cada archivo (`/PageLabels`): hoy la salida numera de corrido.
- **Capas** (`/OCProperties`): habría que fusionar los grupos de contenido opcional de cada origen.
- **Estructura de accesibilidad** (PDF etiquetado, `/StructTreeRoot`): hoy se descarta. Fusionarla bien es el trabajo más grande de la lista.

## Distribución

- Hecho: la CI ([`ci.yml`](../.github/workflows/ci.yml)) corre en cada push todas las pruebas, con los PDF de prueba, y `all.ps1` sin corpus; la release 1.0.0 está lista para publicar ([RELEASE.md](RELEASE.md)).
- Falta: publicarla, y ver la primera corrida de la CI en GitHub (se simuló en la PC, no corrió en un runner de verdad).
