// SPDX-License-Identifier: Apache-2.0
package benchobserve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCPUUsageCounterRequiresCompleteValidValue(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
		bad   bool
	}{
		{"usage_usec 1234\nuser_usec 1000\nsystem_usec 234\n", 1234000, false},
		{"user_usec 1000\n", 0, true}, {"usage_usec wrong\n", 0, true},
		{"usage_usec -1\n", 0, true}, {"usage_usec 18446744073709551615\n", 0, true},
	} {
		file := filepath.Join(t.TempDir(), "cpu.stat")
		if err := os.WriteFile(file, []byte(tc.value), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := (&CPUCounter{file: file}).Read()
		if (err != nil) != tc.bad || err == nil && got != tc.want {
			t.Fatalf("value %q: %d %v", tc.value, got, err)
		}
	}
}

func TestCPUCounterRejectsSharedOrInvalidCgroup(t *testing.T) {
	for _, value := range []string{"0::/user.slice/terminal.scope\n", "0::/../sourceaec-bench-escape.scope\n", "1:cpu:/legacy\n"} {
		if _, err := cpuStatPath(value); err == nil {
			t.Fatalf("accepted unisolated or invalid group %q", value)
		}
	}
	got, err := cpuStatPath("0::/user.slice/sourceaec-bench-native.scope\n")
	if err != nil || got != "/sys/fs/cgroup/user.slice/sourceaec-bench-native.scope/cpu.stat" {
		t.Fatal(got, err)
	}
}
