package extractor

import "testing"

func TestExtractOntInterfaces(t *testing.T) {
	input := `
typ:devops># show equipment ont interface
===============================================================================
interface table
===============================================================================
ont-idx         |eqpt-ver-num  |sw-ver-act    |actual  |version-number|sernum      |yp-serial-no             |cfgfile1      |cfgfile2
----------------+--------------+--------------+--------+--------------+------------+-------------------------+--------------+--------------
1/1/1/1         3FE49937AAAA01 3FE49568HJJ186 2        3FE49937AAAA01 ALCL:FC6344ED ALCLFC6344ED
1/1/1/2         3FE49937AAAA01 3FE49568LJLJ42 2        3FE49937AAAA01 ALCL:FC63404D ALCLFC63404D
1/1/1/30        3TN00673BDAA01 3TN00702HJM195 2        3TN00673BDAA01 ALCL:B46769C4 B46769C4                 CFGALCL001
`

	got := ExtractOntInterfaces(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 rows, got %d: %#v", len(got), got)
	}
	if got[0].OntIdx != "1/1/1/1" || got[0].EqptVerNum != "3FE49937AAAA01" || got[0].SwVerAct != "3FE49568HJJ186" {
		t.Fatalf("unexpected first row: %#v", got[0])
	}
	if got[0].SerNum != "ALCL:FC6344ED" || got[0].YpSerialNo != "ALCLFC6344ED" {
		t.Fatalf("unexpected serial fields: %#v", got[0])
	}
	if got[2].CfgFile1VerAct != "CFGALCL001" {
		t.Fatalf("expected cfgfile1 to parse, got %#v", got[2])
	}
}
