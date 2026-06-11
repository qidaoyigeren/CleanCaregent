package compatibility

import "testing"

func TestMatrixReturnsThreeStates(t *testing.T) {
	matrix := NewDefaultMatrix()
	if got := matrix.Check("P400", "F400").Status; got != Compatible {
		t.Fatalf("P400/F400 status = %s", got)
	}
	if got := matrix.Check("P400", "F500").Status; got != Incompatible {
		t.Fatalf("P400/F500 status = %s", got)
	}
	if got := matrix.Check("P400", "F999").Status; got != Unknown {
		t.Fatalf("P400/F999 status = %s", got)
	}
}
