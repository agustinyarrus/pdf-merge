package tui

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// La capa de consola y el propio pdf-merge son livianos a propósito: no cargan
// red (net, net/netip) ni lanzar procesos (os/exec), que no necesitan y
// pesarían en el .exe. Si un cambio los arrastra, este test lo dice.
func TestSinPaquetesPesados(t *testing.T) {
	if testing.Short() {
		t.Skip("-short: sin correr go list")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go no está en el PATH")
	}
	out, err := exec.Command("go", "list", "-deps", "github.com/agustinyarrus/pdf-merge/internal/tui", "github.com/agustinyarrus/pdf-merge").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	deps := strings.Fields(string(out))
	for _, no := range []string{"net", "net/netip", "os/exec"} {
		if slices.Contains(deps, no) {
			t.Errorf("tui o pdf-merge dependen de %s", no)
		}
	}
}
