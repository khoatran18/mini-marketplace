package playbooks

import (
	"os"
	"path/filepath"
	"testing"
)

const realDir = "../../../../docs/playbooks"

// Mọi file trong docs/playbooks phải đọc được với lược đồ chặt (khoá lạ là lỗi) và id khớp tên file.
func TestRealPlaybooksLoad(t *testing.T) {
	lib, err := Load(realDir)
	if err != nil {
		t.Fatal(err)
	}
	all := lib.List("")
	if len(all) != 10 {
		t.Fatalf("want 10 playbooks, got %d", len(all))
	}
	for _, s := range all {
		p, ok := lib.Get(s.ID)
		if !ok || len(p.Diagnose) == 0 || len(p.Causes) == 0 || len(p.Triggers.Alerts) == 0 {
			t.Errorf("%s: thiếu diagnose, causes hoặc alerts", s.ID)
		}
		for _, d := range p.Diagnose {
			if !d.ReadOnly {
				t.Errorf("%s/%s: bước chẩn đoán phải chỉ đọc", s.ID, d.ID)
			}
		}
	}
}

func TestListByAlertAndGet(t *testing.T) {
	lib, err := Load(realDir)
	if err != nil {
		t.Fatal(err)
	}
	got := lib.List("outboxdeadrows")
	if len(got) != 1 || got[0].ID != "outbox-stuck" {
		t.Fatalf("alert lookup: %+v", got)
	}
	if len(lib.List("NoSuchAlert")) != 0 {
		t.Fatal("unknown alert must give an empty list")
	}
	if _, ok := lib.Get("nope"); ok {
		t.Fatal("unknown id must not be found")
	}
}

func TestGroundTruthIsNeverServed(t *testing.T) {
	lib, _ := Load(realDir)
	p, _ := lib.Get("outbox-stuck")
	if len(p.GroundTruth) == 0 {
		t.Fatal("test premise: outbox-stuck has ground_truth in YAML")
	}
	b, _ := jsonOf(p)
	if contains(b, "ground_truth") {
		t.Fatal("ground_truth leaked into JSON")
	}
}

func TestSchemaDriftAndBadFilesAreErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.yml", "id: a\ntitle: A\ntriggers: {alerts: [X]}\nsurprise: 1\n")
	if _, err := Load(dir); err == nil {
		t.Fatal("unknown key must fail")
	}
	write("a.yml", "id: b\ntitle: A\n")
	if _, err := Load(dir); err == nil {
		t.Fatal("id not matching the file name must fail")
	}
	if lib, err := Load(filepath.Join(dir, "missing")); err != nil || len(lib.List("")) != 0 {
		t.Fatalf("missing dir = empty library, got %v %v", lib, err)
	}
}
