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

- Releases en GitHub con el `pdf-merge.exe` que genera `build.ps1` y su SHA256.
- Integración continua: `go vet` y `go test` en `windows-latest` con GitHub Actions; con Python en el runner, también `scripts\fixtures.ps1` y `all.ps1` sin corpus (no necesitan nada de la máquina).
