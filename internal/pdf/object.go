// Package pdf lee y reescribe archivos PDF lo suficiente para combinarlos: un
// modelo de objetos completo, un parser que entiende tanto las tablas xref
// clásicas como los xref streams y object streams (PDF 1.5+), y un escritor que
// emite un PDF nuevo con numeración fresca. Todo en Go puro (compress/zlib para
// los flujos). No renderiza: copia el contenido de las páginas tal cual.
package pdf

import (
	"math"
	"sort"
	"strconv"
)

// Object es cualquier valor del árbol de objetos de un PDF.
type Object interface{ isObject() }

// Null es el objeto nulo.
type Null struct{}

// Bool es true/false.
type Bool bool

// Integer es un entero.
type Integer int64

// Real es un número con coma.
type Real float64

// String es una cadena (literal "(...)" o hexadecimal "<...>"). Se guarda ya
// decodificada; el escritor la vuelve a serializar de forma segura.
type String struct {
	Value []byte
	Hex   bool // preferencia de serialización; el contenido es el mismo
}

// Name es un nombre PDF (/Type), sin la barra.
type Name string

// Array es una lista ordenada de objetos.
type Array []Object

// Dict es un diccionario de nombre→objeto.
type Dict map[Name]Object

// Stream es un diccionario con datos crudos asociados.
type Stream struct {
	Dict Dict
	Raw  []byte // bytes tal como estaban entre stream/endstream (sin decodificar)

	// Cuando /Length es una referencia indirecta el scanner no puede
	// resolverla en el momento y corta en el primer "endstream" (que podría
	// aparecer por azar dentro de datos binarios). Se guarda dónde empezaban
	// los datos para que el Document los recorte con la longitud verdadera.
	lenRef    *Ref
	dataStart int
	file      []byte

	// Si el documento está cifrado, los bytes se descifran al leerlos (una
	// vez, con memo) con la clave del objeto que los contiene.
	encrypted          bool
	cryptNum, cryptGen int
	plain              []byte
}

// Ref es una referencia indirecta "n g R".
type Ref struct {
	Num, Gen int
}

func (Null) isObject()    {}
func (Bool) isObject()    {}
func (Integer) isObject() {}
func (Real) isObject()    {}
func (String) isObject()  {}
func (Name) isObject()    {}
func (Array) isObject()   {}
func (Dict) isObject()    {}
func (*Stream) isObject() {}
func (Ref) isObject()     {}

// Get devuelve la entrada key del diccionario (nil si no está).
func (d Dict) Get(key Name) Object { return d[key] }

// GetName devuelve una entrada como Name resuelta a texto, o "".
func (d Dict) GetName(key Name) string {
	if n, ok := d[key].(Name); ok {
		return string(n)
	}
	return ""
}

// intValue extrae un entero de un Integer o Real.
func intValue(o Object) (int, bool) {
	switch v := o.(type) {
	case Integer:
		return int(v), true
	case Real:
		return int(v), true
	}
	return 0, false
}

// sortedKeys devuelve las claves del diccionario ordenadas, para una salida
// determinista (útil para pruebas reproducibles).
func (d Dict) sortedKeys() []Name {
	keys := make([]Name, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// formatReal serializa un Real sin notación científica (la sintaxis PDF no la
// admite) y SIN PERDER PRECISIÓN: la representación más corta que vuelve
// exactamente al mismo float64. Con un redondeo a 6 decimales, la /FontMatrix
// de 1/2048 (0.00048828125) de las fuentes Type 3 que genera Chrome quedaba en
// 0.000488 y los glifos se dibujaban un 0,06 % más chicos.
func formatReal(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "0"
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if s == "-0" {
		return "0"
	}
	return s
}
