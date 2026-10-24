// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"os"
	"testing"
)

func TestNativeRealGeometryLifecycle(t *testing.T) {
	if os.Getenv("SOURCEAEC_TEST_GEOMETRY") != "1" {
		t.Skip("set SOURCEAEC_TEST_GEOMETRY=1 with an external geometry interpreter")
	}
	scripts := map[string]string{
		"ifc": `PARAMS = {"name": "synthetic wall"}
import ifcopenshell
import sys
model = ifcopenshell.file(schema="IFC4")
model.create_entity("IfcWall", GlobalId="0aaaaaaaaaaaaaaaaaaaaa", Name=PARAMS["name"])
model.write(sys.argv[1])
`,
		"dxf": `PARAMS = {"x": 3.0}
import ezdxf
import sys
doc = ezdxf.new()
doc.modelspace().add_line((0, 0), (PARAMS["x"], 1))
doc.saveas(sys.argv[1])
`,
	}
	for kind, script := range scripts {
		t.Run(kind, func(t *testing.T) {
			s, _ := testService(t, kind)
			s.Executor = NewSandbox()
			invoke(t, s, "PUT", "script", map[string]any{"script": script})
			invoke(t, s, "POST", "script/run", nil)
			result := invoke(t, s, "POST", "script/save", nil).(map[string]any)
			if result["version"] != "v1" {
				t.Fatal(result)
			}
			invoke(t, s, "POST", "script/rollback", map[string]any{"version": "v1"})
		})
	}
}

func TestManagedExternalEnvironmentProducesReusableIdentity(t *testing.T) {
	if os.Getenv("SOURCEAEC_TEST_GEOMETRY") != "1" {
		t.Skip("external runtime integration disabled")
	}
	s := NewSandbox()
	s.Python = ""
	s.CacheDir = t.TempDir()
	source := "# /// script\n# dependencies = []\n# ///\nPARAMS = {}\nimport sys\nopen(sys.argv[1], 'w').write(" + pythonLiteral(string(fixtureIFC("managed"))) + ")\n"
	first, err := s.Run(context.Background(), source, "ifc")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Run(context.Background(), source, "ifc")
	if err != nil {
		t.Fatal(err)
	}
	if first.Environment == "" || first.Environment != second.Environment {
		t.Fatal("environment identity drift")
	}
}
