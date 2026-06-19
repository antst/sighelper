package env

import (
	"testing"
	"time"
)

func TestOr(t *testing.T) {
	t.Setenv("SIGHELPER_X", "set")
	if Or("SIGHELPER_X", "def") != "set" {
		t.Fatal("want set value")
	}
	if Or("SIGHELPER_UNSET_XYZ", "def") != "def" {
		t.Fatal("want default")
	}
}

func TestBool(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "True", "yes", "on"} {
		t.Setenv("SIGHELPER_B", v)
		if !Bool("SIGHELPER_B") {
			t.Fatalf("%q should be truthy", v)
		}
	}
	for _, v := range []string{"", "0", "false", "no", "nope"} {
		t.Setenv("SIGHELPER_B", v)
		if Bool("SIGHELPER_B") {
			t.Fatalf("%q should be falsy", v)
		}
	}
}

func TestDuration(t *testing.T) {
	t.Setenv("SIGHELPER_D", "300ms")
	if got := Duration("SIGHELPER_D", time.Second); got != 300*time.Millisecond {
		t.Fatalf("got %v", got)
	}
	t.Setenv("SIGHELPER_D", "garbage")
	if got := Duration("SIGHELPER_D", time.Second); got != time.Second {
		t.Fatalf("invalid value should fall back to default, got %v", got)
	}
	t.Setenv("SIGHELPER_D", "")
	if got := Duration("SIGHELPER_D", 250*time.Millisecond); got != 250*time.Millisecond {
		t.Fatalf("unset should use default, got %v", got)
	}
}
