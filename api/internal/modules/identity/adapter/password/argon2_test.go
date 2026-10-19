package password

import (
	"strings"
	"testing"
)

func TestHashVerify(t *testing.T) {
	h := &Argon2id{P: Params{Memory: 8 * 1024, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16}}
	enc, err := h.Hash("mật-khẩu-đủ-dài")
	if err != nil || !strings.HasPrefix(enc, "$argon2id$v=19$m=8192,t=1,p=1$") {
		t.Fatalf("%v %s", err, enc)
	}
	if ok, _ := h.Verify("mật-khẩu-đủ-dài", enc); !ok {
		t.Fatal("verify failed")
	}
	if ok, _ := h.Verify("wrong", enc); ok {
		t.Fatal("wrong password accepted")
	}
	enc2, _ := h.Hash("mật-khẩu-đủ-dài")
	if enc == enc2 {
		t.Fatal("salt not random")
	}
	if _, err := h.Verify("x", "$bcrypt$..."); err == nil {
		t.Fatal("malformed accepted")
	}
}
