package docparser

import "testing"

func TestBuiltinEngineSupportsFreeMind(t *testing.T) {
	engine := &builtinEngine{}
	for _, ft := range engine.FileTypes(true) {
		if ft == "mm" {
			return
		}
	}
	t.Fatal("expected builtin parser engine to advertise .mm FreeMind support")
}
