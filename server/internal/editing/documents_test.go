// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIFCDiffIgnoresSTEPIdentifiers(t *testing.T) {
	base := fixtureIFC("wall")
	target := []byte(strings.ReplaceAll(string(base), "#1=", "#999="))
	diff, e := Compare(base, target, "ifc")
	if e != nil || len(diff["changed"].([]map[string]any)) != 0 {
		t.Fatal(diff, e)
	}
	diff, e = Compare(base, fixtureIFC("changed"), "ifc")
	if e != nil || len(diff["changed"].([]map[string]any)) != 1 {
		t.Fatal(diff, e)
	}
}
func TestDXFRenderAndDiff(t *testing.T) {
	render, e := renderDXF(fixtureDXF(3))
	if e != nil {
		t.Fatal(e)
	}
	entries := render["entities"].([]map[string]any)
	if len(entries) != 1 || entries[0]["key"] != "wall" || entries[0]["type"] != "LINE" {
		t.Fatal(render)
	}
	diff, e := Compare(fixtureDXF(1), fixtureDXF(3), "dxf")
	if e != nil || len(diff["changed"].([]map[string]any)) != 1 {
		t.Fatal(diff, e)
	}
}
func TestScriptDiff(t *testing.T) {
	result := scriptChanges("PARAMS = {'a':1}\n", "PARAMS = {'a':2}\n", "v1", "v2")
	if len(result["params_changes"].([]map[string]any)) != 1 || result["stats"].(map[string]int)["added"] != 1 {
		t.Fatal(result)
	}
}
func TestDependenciesPinnedAndDataOnly(t *testing.T) {
	for _, text := range []string{"# /// script\n# dependencies = ['evil @ file:///tmp/evil']\n# ///", "# /// script\n# dependencies = ['ezdxf>=1.0']\n# ///"} {
		if _, e := dependencies(text, "dxf"); e == nil {
			t.Fatal("accepted unsafe dependency", text)
		}
	}
	if _, e := dependencies("# /// script\n# dependencies = ['ezdxf==1.4.3']\n# ///", "dxf"); e != nil {
		t.Fatal(e)
	}
}
func TestProductsRejectSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "private")
	if e := os.WriteFile(target, []byte("private"), 0600); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(root, "out.ifc")
	if e := os.Symlink(target, link); e != nil {
		t.Fatal(e)
	}
	if _, e := readProduct(link, productLimit); e == nil {
		t.Fatal("read symlink product")
	}
}
func TestSandboxMissingBwrapFailsClosed(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	s := NewSandbox()
	_, e := s.Run(context.Background(), "PARAMS = {}", "ifc")
	if e == nil {
		t.Fatal("missing bwrap accepted")
	}
}

func TestIFCPropertyChangeReachesRelatedObject(t *testing.T) {
	base := "ISO-10303-21;HEADER;ENDSEC;DATA;" +
		"#1=IFCWALL('0aaaaaaaaaaaaaaaaaaaaa',$,'wall',$,$,$,$,$);" +
		"#2=IFCPROPERTYSINGLEVALUE('Rating',$,IFCLABEL('old'),$);" +
		"#3=IFCPROPERTYSET('0bbbbbbbbbbbbbbbbbbbbb',$,'Pset_Test',$,(#2));" +
		"#4=IFCRELDEFINESBYPROPERTIES('0ccccccccccccccccccccc',$,$,$,(#1),#3);ENDSEC;END-ISO-10303-21;"
	diff, err := Compare([]byte(base), []byte(strings.ReplaceAll(base, "'old'", "'new'")), "ifc")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range diff["changed"].([]map[string]any) {
		if entry["guid"] == "0aaaaaaaaaaaaaaaaaaaaa" {
			found = true
		}
	}
	if !found {
		t.Fatal("property change was not attributed to wall", diff)
	}
}

func TestSourceArgumentEditsPreserveSurroundingCode(t *testing.T) {
	script := "PARAMS = {'wall': {'name': 'before'}}\n# keep me\nwall = make_wall(\n    name=params['wall']['name'],  # keep comment\n    height=3.0,\n)\n"
	changed, err := rewriteArgument(script, 3, "name", "after", "parameter")
	if err != nil {
		t.Fatal(err)
	}
	params, _, _, err := extractParams(changed)
	if err != nil || params["wall"].(map[string]any)["name"] != "after" {
		t.Fatal(changed, err)
	}
	changed, err = rewriteArgument(script, 3, "height", 4.5, "override")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(changed, "height=4.5,") || !strings.Contains(changed, "# keep comment") {
		t.Fatal(changed)
	}
	if _, err = rewriteArgument(script, 3, "height", 4.5, "parameter"); err == nil {
		t.Fatal("accepted unbound parameter")
	}
}

func TestAmbiguousCallsitesAreNotRewritten(t *testing.T) {
	script := "PARAMS = {}\na = first(name='one'); b = second(name='two')\n"
	if _, err := rewriteArgument(script, 2, "name", "wrong", "override"); err == nil {
		t.Fatal("ambiguous line rewritten")
	}
}
func TestDuplicateParamsCannotDivergeFromExecution(t *testing.T) {
	if _, _, _, err := extractParams("PARAMS = {'a':1}\nPARAMS = {'a':2}\n"); err == nil {
		t.Fatal("duplicate PARAMS accepted")
	}
}
