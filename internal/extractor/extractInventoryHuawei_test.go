package extractor

import (
	"testing"
)

// Sample from real Huawei "display ont version 0 all" terminal output.
const sampleOutput = `  F/S/P/ONT-ID  Vendor  ONT Model             Software Version   Customized Vendors
----------------------------------------------------------------------------------
  0/ 4/ 2/ 29   HWTC    OG-976V2                  V3.1.0-160815     -      
  0/ 4/ 3/  0   HWTC    OG-976V                   V3.1.0-160815     -      
  0/ 4/ 3/  1   HWTC    OG-976V                   V3.1.0-160815     -      
  0/ 5/ 2/  4   HWTC    HG8145V6                  V5R023C00S252     -      
  0/ 6/ 4/ 24   HWTC    120C                      V5R019C10S125     -      
  -----------------------------------------------------------------------------
  The total of online ONTs are: 1128
  -----------------------------------------------------------------------------
MA5600T#`

func TestExtractHuaweiOntVersions(t *testing.T) {
	got := ExtractHuaweiOntVersions(sampleOutput)
	if len(got) < 4 {
		t.Fatalf("expected at least 4 ONTs, got %d", len(got))
	}
	// Check first
	if got[0].Index != "0/4/2/29" {
		t.Errorf("first Index: got %q", got[0].Index)
	}
	if got[0].OntModel != "OG-976V2" {
		t.Errorf("first OntModel: got %q", got[0].OntModel)
	}
	if got[0].SwVersion != "V3.1.0-160815" {
		t.Errorf("first SwVersion: got %q", got[0].SwVersion)
	}
	// Check HG8145V6
	var found bool
	for _, o := range got {
		if o.OntModel == "HG8145V6" {
			found = true
			if o.Index != "0/5/2/4" {
				t.Errorf("HG8145V6 Index: got %q", o.Index)
			}
			break
		}
	}
	if !found {
		t.Error("HG8145V6 not found")
	}
	// Check 120C (model with digits)
	for _, o := range got {
		if o.OntModel == "120C" {
			found = true
			break
		}
	}
}
