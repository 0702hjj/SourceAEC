package agent

import (
	"context"
	"testing"

	localbk "github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"os"
	"path/filepath"
)

// The maintained catalog provides one integration entry, independent of model roles.
func TestMaintainedCatalogHasSingleIntegrationSkill(t *testing.T) {
	if _, err := os.Stat(maintainedSkillsDir()); err != nil {
		t.Fatalf("maintained skill directory missing: %v", err)
	}
	ctx := context.Background()
	backend, err := localbk.NewBackend(ctx, &localbk.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sb, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: backend,
		BaseDir: maintainedSkillsDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	fms, err := sb.List(ctx)
	if err != nil {
		t.Fatalf("List maintained skills: %v", err)
	}

	names := map[string]bool{}
	for _, fm := range fms {
		names[fm.Name] = true
	}
	if len(fms) != 1 || !names["sourceaec"] {
		t.Fatalf("expected only SourceAEC integration, got %v", names)
	}
}

func maintainedSkillsDir() string {
	path, _ := filepath.Abs("../../../skills")
	return path
}
