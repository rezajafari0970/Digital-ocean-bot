package provisioning

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryHeadroomFreshReadRecoveryAndIsolation(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root ownership checks require root")
	}
	for _, tc := range []string{"healthy", "existing_swap", "lost_activation_response", "unmanaged_file"} {
		t.Run(tc, func(t *testing.T) {
			dir := t.TempDir()
			root := filepath.Join(dir, "managed")
			mem := filepath.Join(dir, "meminfo")
			swaps := filepath.Join(dir, "swaps")
			fstab := filepath.Join(dir, "fstab")
			log := filepath.Join(dir, "calls")
			write := func(p, s string, mode os.FileMode) {
				t.Helper()
				if e := os.WriteFile(p, []byte(s), mode); e != nil {
					t.Fatal(e)
				}
			}
			memory := "MemTotal: 453000 kB\nSwapTotal: 0 kB\n"
			if tc == "healthy" {
				memory = "MemTotal: 2048000 kB\nSwapTotal: 0 kB\n"
			}
			if tc == "existing_swap" {
				memory = "MemTotal: 453000 kB\nSwapTotal: 1048576 kB\n"
			}
			write(mem, memory, 0600)
			write(swaps, "Filename Type Size Used Priority\n", 0600)
			write(fstab, "# keep unrelated mounts\n", 0600)
			if tc == "unmanaged_file" {
				os.Mkdir(root, 0700)
				write(filepath.Join(root, "swap"), "do not overwrite", 0600)
			}
			bin := filepath.Join(dir, "bin")
			os.Mkdir(bin, 0700)
			write(filepath.Join(bin, "blkid"), "#!/bin/bash\nlast=\""+root+"/swap.formatted\"\nif [ -f \"$last\" ]; then echo swap; else exit 2; fi\n", 0700)
			write(filepath.Join(bin, "mkswap"), "#!/bin/bash\necho mkswap >> '"+log+"'\ntouch \"$1.formatted\"\n", 0700)
			// Activation commits but its response is lost. A second run must observe
			// /proc/swaps, avoid a second activation/format, and finish fstab persistence.
			write(filepath.Join(bin, "swapon"), "#!/bin/bash\necho swapon >> '"+log+"'\nprintf 'Filename Type Size Used Priority\\n%s file 4 0 -2\\n' \"$1\" > '"+swaps+"'\nexit 1\n", 0700)
			cmd := MemoryHeadroomCommand()
			replacements := map[string]string{"/var/lib/digital-ocean-bot-memory": root, "/proc/meminfo": mem, "/proc/swaps": swaps, "/etc/fstab": fstab, "SIZE=1024**3": "SIZE=4096", "2*1024**3": "8192", "1024**3+max": "4096+max"}
			for from, to := range replacements {
				cmd = strings.ReplaceAll(cmd, from, to)
			}
			run := func() error {
				c := exec.Command("bash", "-c", cmd)
				c.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
				out, e := c.CombinedOutput()
				if e != nil {
					t.Log(string(out))
				}
				return e
			}
			err := run()
			if tc == "healthy" || tc == "existing_swap" {
				if err != nil {
					t.Fatal(err)
				}
				if _, e := os.Stat(root); !os.IsNotExist(e) {
					t.Fatal("sufficient-memory host mutated")
				}
				return
			}
			if tc == "unmanaged_file" {
				if err == nil {
					t.Fatal("unmanaged file accepted")
				}
				b, _ := os.ReadFile(filepath.Join(root, "swap"))
				if string(b) != "do not overwrite" {
					t.Fatal("unmanaged file changed")
				}
				return
			}
			if err == nil {
				t.Fatal("lost activation response not simulated")
			}
			if e := run(); e != nil {
				t.Fatal("recovery failed", e)
			}
			if e := run(); e != nil {
				t.Fatal("idempotent verification failed", e)
			}
			b, _ := os.ReadFile(log)
			if string(b) != "mkswap\nswapon\n" {
				t.Fatal("blind mutation retry", string(b))
			}
			b, _ = os.ReadFile(fstab)
			if strings.Count(string(b), root+"/swap") != 1 || !strings.Contains(string(b), "# keep unrelated mounts") {
				t.Fatal("fstab corruption")
			}
			b, _ = os.ReadFile(filepath.Join(root, "state.json"))
			if !strings.Contains(string(b), `"state": "ACTIVE"`) {
				t.Fatal("activation not journaled")
			}
		})
	}
}
