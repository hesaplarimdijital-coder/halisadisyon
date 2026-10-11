package store

import (
	"path/filepath"
	"testing"
	"time"
)

// Yedek, ana bağlantı meşgulken (uzun bir yazma işlemi sürerken) de alınabilmeli;
// yani siparişleri bekletmeyen ayrı bir okuma bağlantısı kullanılmalı.
func TestBackupDoesNotNeedMainConnection(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "adisyon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO settings(key,value) VALUES('once','1')`); err != nil {
		t.Fatal(err)
	}
	tx, err := st.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES('sonra','2')`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- st.BackupTo(filepath.Join(dir, "yedek.db")) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("yedek ana bağlantıyı bekledi")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	b, err := Open(filepath.Join(dir, "yedek.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var n int
	b.DB.QueryRow(`SELECT COUNT(*) FROM settings WHERE key IN ('once','sonra')`).Scan(&n)
	if n != 1 {
		t.Fatalf("yedek tutarlı anlık görüntü değil: %d satır", n)
	}
}

func TestLineIDsAreNotReused(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "adisyon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(nil); err != nil {
		t.Fatal(err)
	}
	st.DB.Exec(`INSERT INTO adisyons(no,business_date,table_no,status,opened_at) VALUES(1,'2026-10-11',1,'open','x')`)
	ins := func() int64 {
		r, err := st.DB.Exec(`INSERT INTO adisyon_lines(adisyon_no,pos,name,unit,price,vat,qty) VALUES(1,1,'Kola','adet',100,10,1)`)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := r.LastInsertId()
		return id
	}
	a := ins()
	st.DB.Exec(`DELETE FROM adisyon_lines WHERE id=?`, a)
	if b := ins(); b == a {
		t.Fatal("silinen kalemin numarası yeniden verildi")
	}
}
