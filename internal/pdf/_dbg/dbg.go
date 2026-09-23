//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/agustinyarrus/pdf-merge/internal/pdf"
)

func main() {
	raw, _ := os.ReadFile(os.Args[1])
	doc, err := pdf.Parse(raw)
	if err != nil {
		fmt.Println("parse err:", err)
		return
	}
	fmt.Println(pdf.Debug(doc))
}
