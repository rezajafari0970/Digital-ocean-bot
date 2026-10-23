package provisioning

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestKernelMemoryGuardRootAliasRequiresMountedIdentity(t *testing.T) {
	for _, kind := range []string{"same_root", "interrupted", "different_device", "duplicate_label", "unmounted_device", "missing_label", "changed_console", "changed_kernel", "extra_root", "lost_response"} {
		t.Run(kind, func(t *testing.T) {
			f := newKernelGuardFixture(t)
			before := "linux /vmlinuz-7.0.0-cloud root=LABEL=cloudimg-rootfs ro console=tty1 console=ttyS0\nlinux /vmlinuz-7.0.0-cloud root=LABEL=cloudimg-rootfs ro recovery nomodeset\n"
			after := strings.ReplaceAll(before, "root=LABEL=cloudimg-rootfs", "root=UUID=verified-root kho=off")
			switch kind {
			case "changed_console":
				after = strings.ReplaceAll(after, "console=ttyS0", "console=ttyS1")
			case "changed_kernel":
				after = strings.ReplaceAll(after, "/vmlinuz-7.0.0-cloud", "/vmlinuz-other")
			case "extra_root":
				after = strings.ReplaceAll(after, "ro ", "ro root=UUID=other ")
			}
			f.write(t, f.grub, before, 0600)
			// Real shell commands exercise resolver arguments, ambiguity and failures.
			blkid := "#!/bin/sh\nset -eu\n[ \"$1\" = '-c' ] && [ \"$2\" = '/dev/null' ] && [ \"$3\" = '-t' ] && [ \"$5\" = '-o' ] && [ \"$6\" = 'device' ] || exit 8\ncase \"$4\" in\n LABEL=cloudimg-rootfs) printf '%s\\n' /dev/vda1"
			if kind == "duplicate_label" {
				blkid += " /dev/vdb1"
			}
			if kind == "missing_label" {
				blkid = "#!/bin/sh\nexit 2\n"
			} else {
				blkid += ";;\n UUID=verified-root) echo "
				if kind == "different_device" {
					blkid += "/dev/vdb1"
				} else {
					blkid += "/dev/vda1"
				}
				blkid += ";;\n *) exit 7;;\nesac\n"
			}
			f.write(t, filepath.Join(f.dir, "bin", "blkid"), blkid, 0700)
			f.write(t, filepath.Join(f.dir, "bin", "lsblk"), "#!/bin/sh\n[ \"$1 $2 $3 $4\" = '-dn -o MAJ:MIN --' ] || exit 8\ncase \"$5\" in /dev/vda1) echo 8:1;; /dev/vdb1) echo 8:17;; *) exit 7;; esac\n", 0700)
			mounted := "8:1"
			if kind == "unmounted_device" {
				mounted = "8:17"
			}
			f.write(t, filepath.Join(f.dir, "bin", "findmnt"), "#!/bin/sh\n[ \"$*\" = '-n -o MAJ:MIN --target /' ] || exit 8\necho '"+mounted+"'\n", 0700)
			generator := "#!/bin/sh\necho called >> '" + f.log + "'\ncat > '" + f.grub + "' <<'GRUB'\n" + after + "GRUB\n"
			if kind == "lost_response" {
				generator += "exit 1\n"
			}
			f.write(t, f.update, generator, 0700)
			if kind == "interrupted" {
				f.generator(t, true, false)
				if _, err := f.run(t, "prepare"); err == nil {
					t.Fatal("initial failure accepted")
				}
				f.write(t, f.update, generator, 0700)
				if out, err := exec.Command(f.update).CombinedOutput(); err != nil {
					t.Fatal(string(out), err)
				}
			}
			good := kind == "same_root" || kind == "interrupted" || kind == "lost_response"
			out, err := f.run(t, "prepare")
			if !good {
				if err == nil {
					t.Fatal("unproven boot change accepted", out)
				}
				return
			}
			if err != nil {
				t.Fatal("same mounted root alias rejected", out, err)
			}
			if out, err = f.run(t, "check"); err != nil {
				t.Fatal(out, err)
			}
			saved, _ := os.ReadFile(filepath.Join(f.root, "grub.before"))
			if string(saved) != before {
				t.Fatal("original evidence replaced")
			}
			calls, _ := os.ReadFile(f.log)
			if out, err = f.run(t, "prepare"); err != nil {
				t.Fatal(out, err)
			}
			again, _ := os.ReadFile(f.log)
			if string(calls) != string(again) {
				t.Fatal("unnecessary regeneration")
			}
		})
	}
}
